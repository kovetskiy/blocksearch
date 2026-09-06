//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func openSearchInput(ctx context.Context, filename string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var fd int
	var err error
	for {
		fd, err = unix.Open(filename, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if !errors.Is(err, unix.EINTR) || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &os.PathError{Op: "open", Path: filename, Err: err}
	}
	owned := true
	defer func() {
		if owned {
			_ = unix.Close(fd)
		}
	}()

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, &os.PathError{Op: "stat", Path: filename, Err: err}
	}
	if stat.Mode&unix.S_IFMT == unix.S_IFIFO && !searchInputDescriptorPath(filename) {
		if err := waitSearchInputFIFO(ctx, fd); err != nil {
			return nil, &os.PathError{Op: "open", Path: filename, Err: err}
		}
	}
	if stat.Mode&unix.S_IFMT == unix.S_IFREG || stat.Mode&unix.S_IFMT == unix.S_IFDIR {
		if err := unix.SetNonblock(fd, false); err != nil {
			return nil, &os.PathError{Op: "open", Path: filename, Err: err}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filename)
	owned = false
	return file, nil
}

func searchInputDescriptorPath(filename string) bool {
	filename = filepath.Clean(filename)
	if filename == "/dev/stdin" {
		return true
	}
	dir := filepath.Dir(filename)
	if dir != "/dev/fd" && dir != "/proc/self/fd" && dir != "/proc/thread-self/fd" &&
		dir != "/proc/"+strconv.Itoa(os.Getpid())+"/fd" {
		return false
	}
	fd, err := strconv.Atoi(filepath.Base(filename))
	return err == nil && fd >= 0
}

func waitSearchInputFIFO(ctx context.Context, fd int) error {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := unix.Poll(fds, 20)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if fds[0].Revents&unix.POLLNVAL != 0 {
			return unix.EBADF
		}
		if fds[0].Revents&unix.POLLERR != 0 {
			return unix.EIO
		}
		if fds[0].Revents&(unix.POLLIN|unix.POLLHUP) != 0 {
			return nil
		}
	}
}

func prepareCLIOutput(ctx context.Context, output *os.File) (*os.File, func(), error) {
	noop := func() {}
	if err := ctx.Err(); err != nil {
		return nil, noop, err
	}
	info, err := output.Stat()
	if err != nil {
		return nil, noop, err
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return output, noop, nil
	}
	raw, err := output.SyscallConn()
	if err != nil {
		return nil, noop, err
	}
	fd := -1
	flags := 0
	var setupErr error
	err = raw.Control(func(original uintptr) {
		flags, setupErr = unix.FcntlInt(original, unix.F_GETFL, 0)
		if setupErr != nil {
			return
		}
		syscall.ForkLock.RLock()
		fd, setupErr = unix.Dup(int(original))
		if setupErr == nil {
			unix.CloseOnExec(fd)
		}
		syscall.ForkLock.RUnlock()
		if setupErr == nil {
			setupErr = unix.SetNonblock(fd, true)
		}
	})
	if err != nil || setupErr != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return nil, noop, errors.Join(err, setupErr)
	}
	file := os.NewFile(uintptr(fd), output.Name())
	restore := func() {
		_ = raw.Control(func(original uintptr) {
			_, _ = unix.FcntlInt(original, unix.F_SETFL, flags)
		})
	}
	if err := file.SetWriteDeadline(time.Time{}); err != nil {
		_ = file.Close()
		restore()
		return nil, noop, err
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = file.Close()
		close(closed)
	})
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if !stop() {
				<-closed
			}
			_ = file.Close()
			restore()
		})
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, noop, err
	}
	return file, cleanup, nil
}
