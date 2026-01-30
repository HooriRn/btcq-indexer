//go:build linux
// +build linux

package websockets

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func epollCreate1(flag int) (fd int, err error) {
	return unix.EpollCreate1(flag)
}

func epollAdd(epfd int, fd int) (err error) {
	return unix.EpollCtl(
		epfd,
		syscall.EPOLL_CTL_ADD,
		fd,
		&unix.EpollEvent{Events: unix.POLLIN | unix.POLLHUP, Fd: int32(fd)})
}

func epollDel(epfd int, fd int) (err error) {
	return unix.EpollCtl(epfd, syscall.EPOLL_CTL_DEL, fd, nil)
}

type epollEvent unix.EpollEvent

func epollWait(epfd int, events []epollEvent, msec int) (n int, err error) {
	var uEvents []unix.EpollEvent
	uEvents = *((*[]unix.EpollEvent)(unsafe.Pointer(&events)))
	return unix.EpollWait(epfd, uEvents, msec)
}
