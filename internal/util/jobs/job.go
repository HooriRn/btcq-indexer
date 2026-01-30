package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

type NamedFunction struct {
	name string
	job  func()
}

type RunningJob struct {
	quitFinished chan struct{}
	name         string
}

func EmptyJob() NamedFunction {
	return NamedFunction{"", nil}
}

func Later(name string, job func()) NamedFunction {
	if name == "" {
		log.Fatal().Msg("Missing job name")
	}
	return NamedFunction{name, job}
}

func Start(name string, job func()) RunningJob {
	ret := RunningJob{quitFinished: make(chan struct{}), name: name}
	go func() {
		job()
		ret.quitFinished <- struct{}{}
	}()
	return ret
}

func (nf NamedFunction) Start() *RunningJob {
	if nf.job == nil {
		return nil
	}
	if nf.name == "" {
		log.Fatal().Msg("Job without name")
	}
	job := Start(nf.name, nf.job)
	return &job
}

func (q *RunningJob) Wait(finishCTX context.Context) error {
	if q == nil || q.quitFinished == nil {
		return nil
	}
	log.Info().Msgf("Waiting %s goroutine to finish", q.name)

	// Golang doesn't support select with preference when both channels already have data,
	// therefore we simulate it with two select statements.
	select {
	case <-q.quitFinished:
		log.Info().Msgf("%s stopped", q.name)
		return nil
	default:
	}

	select {
	case <-q.quitFinished:
		log.Info().Msgf("%s stopped", q.name)
		return nil
	case <-finishCTX.Done():
		log.Error().Msgf("Failed to stop %s goroutine within timeout", q.name)
		return fmt.Errorf("Failed to stop %s goroutine within timeout", q.name)
	}
}

func WaitAll(finishCTX context.Context, allJobs ...*RunningJob) {
	allOK := true
	for _, job := range allJobs {
		err := job.Wait(finishCTX)
		if err != nil {
			allOK = false
		}
	}
	if !allOK {
		log.Error().Msg("Failed to finish all jobs")

	}
}

func (q *RunningJob) MustWait() {
	<-q.quitFinished
}

// Sleep is like time.Sleep, but responds to context cancellation
func Sleep(ctx context.Context, delay time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}
