CFLAGS	=-D_GNU_SOURCE
GO	?=go

.PHONY: all
all: qo

.PHONY: clean
clean:
	$(RM) *.o 3p/*.o libquickjs.a libqojs.a qo qjsc

qo: libquickjs.a libqojs.a main.go
	$(GO) build

qjsc: 3p/qjsc.o 3p/quickjs-libc.o libquickjs.a
	$(CC) -o $@ $^ -lm

core.c: core.js qjsc
	./qjsc -p qojs_ -o $@ $<

libqojs.a: core.o
	$(AR) crs $@ $^

libquickjs.a: 3p/cutils.o 3p/libbf.o 3p/libregexp.o 3p/libunicode.o 3p/quickjs.o
	$(AR) crs $@ $^
