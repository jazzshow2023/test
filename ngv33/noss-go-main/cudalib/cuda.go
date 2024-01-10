package cudalib

/*
#include <stdlib.h>
char *solve_noss(char *input, int difficulty);
#cgo LDFLAGS: -L/usr/local/cuda/lib64 -L. -L./ -L../ -lnoss  -lcuda -lcudart -lstdc++
*/
import "C"

import (
	"unsafe"
)

func SolveNoss(event string, difficulty int) string {
	input := GoStringToCCharArray(event)
	nonce := C.solve_noss(&input[0], C.int(difficulty))
	defer C.free(unsafe.Pointer(nonce))
	result := C.GoString(nonce)
	return result
}
func GoStringToCCharArray(goStr string) []C.char {
	cStr := C.CString(goStr)
	defer C.free(unsafe.Pointer(cStr))

	cArray := make([]C.char, len(goStr)+1) // +1 用于空字符

	for i := range cArray {
		cArray[i] = *(*C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(cStr)) + uintptr(i)))
	}

	return cArray
}
