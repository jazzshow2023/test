#include "sha256.cuh"

#include <chrono>
#include <cstdio>
#include <cstring>
#include <cuda_runtime.h>
#include <curand.h>
#include <curand_kernel.h>
#include <device_launch_parameters.h>
#include <fstream>
#include <iomanip>
#include <iostream>
#include <ostream>
#include <stdlib.h>
#include <string>
#include <unistd.h>

#define SHOW_INTERVAL_MS 2000
#define BLOCK_SIZE 256
#define SHA_PER_ITERATIONS 1'048'576
// #define SHA_PER_ITERATIONS 67'108'864
// #define NUMBLOCKS (SHA_PER_ITERATIONS + BLOCK_SIZE - 1) / BLOCK_SIZE
#define NUMBLOCKS (SHA_PER_ITERATIONS + BLOCK_SIZE - 1) / BLOCK_SIZE

static size_t difficulty = 1;

static uint64_t nonce = 0;
static uint64_t user_nonce = 0;
static uint64_t last_nonce_since_update = 0;

// Last timestamp we printed debug infos
static std::chrono::high_resolution_clock::time_point t_last_updated;
__device__ bool checkZeroPadding(unsigned char *sha, uint8_t difficulty) {

  bool isOdd = difficulty % 2 != 0;
  uint8_t max = (difficulty / 2) + 1;

  /*
          Odd : 00 00 01 need to check 0 -> 2
          Even : 00 00 00 1 need to check 0 -> 3
          odd : 5 / 2 = 2 => 2 + 1 = 3
          even : 6 / 2 = 3 => 3 + 1 = 4
  */
  for (uint8_t cur_byte = 0; cur_byte < max; ++cur_byte) {
    uint8_t b = sha[cur_byte];
    if (cur_byte < max - 1) { // Before the last byte should be all zero
      if (b != 0)
        return false;
    } else if (isOdd) {
      if (b > 0x0F || b == 0)
        return false;
    } else if (b <= 0x0f)
      return false;
  }

  return true;
}
__device__ void generateRandomString(char *nonce) {
  curandState_t state;
  curand_init(clock64(), threadIdx.x, 0, &state); // 初始化随机数生成器

  int length = 10;
  const char charset[] = "abcdefghijklmnopqrstuvwxyz0"
                         "123456789"; // 字符集
  for (int i = 0; i < length; i++) {
    int index = curand_uniform(&state) * (sizeof(charset) - 1); // 生成随机索引
    nonce[i] = charset[index];
  }
  nonce[length] = '\0'; //
}

__global__ void sha256_kernel(char *out_input_string_nonce,
                              unsigned char *out_found_hash, int *out_found,
                              char *result_nonce, const char *in_input_string,
                              size_t in_input_string_size, uint8_t difficulty,
                              int offset) {

  uint64_t idx = blockIdx.x * blockDim.x + threadIdx.x;
  char nonce[10 + 1];
  generateRandomString(nonce);
  // if (*out_found == 1) {
  //   return;
  // }
  unsigned char sha[32];
  {
    SHA256_CTX ctx;
    sha256_init(&ctx);
    sha256_update(&ctx, in_input_string, in_input_string_size, nonce, offset);
    sha256_final(&ctx, sha);
  }

  if (checkZeroPadding(sha, difficulty) && atomicExch(out_found, 1) == 0) {
    memcpy(result_nonce, nonce, 10 + 1);
    memcpy(out_found_hash, sha, 32);
  }
}

void pre_sha256() {
  checkCudaErrors(cudaMemcpyToSymbol(dev_k, host_k, sizeof(host_k), 0,
                                     cudaMemcpyHostToDevice));
}

// Prints a 32 bytes sha256 to the hexadecimal form filled with zeroes
void print_hash(const unsigned char *sha256) {
  for (uint8_t i = 0; i < 32; ++i) {
    std::cout << std::hex << std::setfill('0') << std::setw(2)
              << static_cast<int>(sha256[i]);
  }
  std::cout << std::dec << std::endl;
}

const char *solve(std::string line, int difficulty) {
  // Init
  cudaSetDevice(0);
  cudaDeviceSetCacheConfig(cudaFuncCachePreferShared);
  // Get Offset
  std::string nonceString = "nonce";
  size_t found = line.find(nonceString);
  if (found == std::string::npos) {
    std::cout << "Substring found at index: " << found << std::endl;
    throw std::runtime_error("No nonce found line is" + line);
  }
  size_t offset = found + nonceString.size() + 3;
  //
  auto start_time = std::chrono::steady_clock::now();
  const size_t input_size = line.size();
  // Input string for the device
  char *device_input = nullptr;
  char *nonces = nullptr;
  char *result_nonce = nullptr;

  // Output string by the device read by host
  char *g_out = nullptr;
  unsigned char *g_hash_out = nullptr;
  int *g_found = nullptr;

  char *result_nonce_host = (char *)malloc(14 * sizeof(char));

  char g_out_host[input_size + 32 + 1];
  unsigned char *g_hash_out_host[13 + 1];
  int g_found_host = 0;

  pre_sha256();

  // Create the input string for the device
  cudaMalloc(&device_input, input_size + 1);

  cudaMemcpy(device_input, line.c_str(), input_size + 1,
             cudaMemcpyHostToDevice);
  cudaMalloc(&result_nonce, 10 + 1);
  cudaMalloc(&g_out, input_size + 32 + 1);
  cudaMalloc(&g_hash_out, 32);
  cudaMalloc(&g_found, sizeof(int));

  cudaMemcpy(g_found, &g_found_host, sizeof(int), cudaMemcpyHostToDevice);
  cudaError_t err_result = cudaGetLastError();
  if (err_result != cudaSuccess) {
    throw std::runtime_error("Device error \n" +
                             std::string(cudaGetErrorString(err_result)));
  }

  while (!g_found_host) {

    fflush(stdout);
    sha256_kernel<<<dim3(1, NUMBLOCKS), BLOCK_SIZE>>>(
        g_out, g_hash_out, g_found, result_nonce, device_input, input_size,
        difficulty, offset);
    cudaMemcpy(&g_found_host, g_found, sizeof(int), cudaMemcpyDeviceToHost);
    cudaError_t err = cudaDeviceSynchronize();
    if (err != cudaSuccess) {
      throw std::runtime_error("Device error");
    }
    const cudaError_t err_result = cudaGetLastError();
    if (err_result != cudaSuccess) {
      throw std::runtime_error("Device error \n" +
                               std::string(cudaGetErrorString(err_result)));
    }
    nonce += NUMBLOCKS * BLOCK_SIZE;
  }
  auto end_time = std::chrono::steady_clock::now();
  auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(
                      end_time - start_time)
                      .count();
  // printf("speed : %f hash/s\n", nonce / (duration / 1000.0));
  // printf("duration: %d ms\n", duration);
  // printf(line.c_str(), result_nonce, difficulty);
  cudaMemcpy(result_nonce_host, result_nonce, 10 + 1, cudaMemcpyDeviceToHost);
  err_result = cudaGetLastError();
  if (err_result != cudaSuccess) {
    throw std::runtime_error("Device error  Cioy errir \n" +
                             std::string(cudaGetErrorString(err_result)));
  }

  // std::string result(result_nonce_host);
  // printf("\nresult nonce: %s\n", result.c_str());

  cudaFree(g_out);
  cudaFree(g_hash_out);
  cudaFree(g_found);
  cudaFree(device_input);

  // cudaDeviceReset();
  // for (int i = 0; i <= line.size(); ++i) {
  //   if (i >= offset && i < offset + 10) {
  //     line[i] = result[i - offset];
  //   }
  // }
  // printf("input is %s \n", line.c_str());
  return result_nonce_host;
}
// int main() {

//   t_last_updated = std::chrono::high_resolution_clock::now();
//   std::ifstream file("test.txt");
//   std::string line;
//   if (file.is_open()) {
//     getline(file, line);
//     file.close();
//   } else {
//     std::cout << "无法打开文件" << std::endl;
//     exit(1);
//   }
//   solve(line, 4);
//   return 0;
// }

extern "C" {
const char *solve_noss(char *input, int difficulty) {
  std::string input_value(input);
  return solve(input_value, difficulty);
}
}