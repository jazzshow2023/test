# GENCODES = -arch=sm_80

CPPFLAGS += -DNDEBUG 
NVCCFLAGS = -c -O2 -std=c++17 \
            -Xcompiler "-Wall -Wextra -Wno-unused-parameter" \
            --expt-relaxed-constexpr \
            --extended-lambda


# noss: noss.o
# 	nvcc $(LDFLAGS) -o $@ $<

libnoss.a: noss.o
	nvcc --ptxas-options=-v --compiler-options '-fPIC' -lib -o $@ $<

noss.o: noss.cu $(wildcard *.cuh *.h ../include/*.cuh ../include/*.h)
	nvcc $(NVCCFLAGS) $(CPPFLAGS) -o $@ $<

.PHONY: clean
clean:
	rm noss noss.o
