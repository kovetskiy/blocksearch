# Makefile: tabs are mandatory for recipe lines. Mixing breaks the build.

.PHONY: all clean test build

CC = gcc
CFLAGS = -Wall -Wextra -O2

all: build test

build:
	$(CC) $(CFLAGS) -o app main.c utils.c

test:
	@echo "running tests"
	./app --self-test

clean:
	rm -f app *.o

install: build
	install -m 0755 app $(DESTDIR)/usr/local/bin/app

# nested call with tabs preserved
dist: clean
	mkdir -p dist
	cp app dist/
	tar czf app.tar.gz dist
	rm -rf dist
