//go:build !linux
// +build !linux

package websockets

func epollCreate1(flag int) (fd int, err error) {
	panic("Implemented only under linux")
}

func epollAdd(epfd int, fd int) (err error) {
	panic("Implemented only under linux")
}

func epollDel(epfd int, fd int) (err error) {
	panic("Implemented only under linux")
}

type epollEvent struct {
	Events uint32
	Fd     int32
	Pad    int32
}

func epollWait(epfd int, events []epollEvent, msec int) (n int, err error) {
	panic("Implemented only under linux")
}
