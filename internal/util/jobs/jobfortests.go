package jobs

import "context"

type JobForTests struct {
	quitFinished chan struct{}
	cancel       context.CancelFunc
}

// For the convenience of tests creates a context underneath.
func StartForTests(job func(context.Context)) JobForTests {
	ctx, cancel := context.WithCancel(context.Background())
	ret := JobForTests{make(chan struct{}), cancel}
	go func() {
		job(ctx)
		ret.quitFinished <- struct{}{}
	}()
	return ret
}

func (q JobForTests) Quit() {
	q.cancel()
	<-q.quitFinished
}
