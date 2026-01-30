package record

func ResetRecorderForTest() {
	Recorder.runningTotals = *newRunningTotals()
}
