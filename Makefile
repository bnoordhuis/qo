GO	?=go

.PHONY: all
all: qo

.PHONY: clean
clean:
	$(RM) 3p/*.o libquickjs.a qo

qo: libquickjs.a main.go
	$(GO) build

libquickjs.a: 3p/cutils.o 3p/libbf.o 3p/libregexp.o 3p/libunicode.o 3p/quickjs.o
	$(AR) crs $@ $^
