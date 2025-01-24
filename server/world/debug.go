package world

import (
	"fmt"
	"runtime"
)

// captureCallerInfo captures the caller information of the function that calls it.
func captureCallerInfo() []string {
	// Capture up to 10 stack frames
	pc := make([]uintptr, 64)
	// Skip the first 2 frames (runtime.Callers and GetCallerInfo itself)
	n := runtime.Callers(2, pc)
	if n == 0 {
		return []string{"No callers found"}
	}

	frames := runtime.CallersFrames(pc[:n])
	var result []string

	for {
		frame, more := frames.Next()
		result = append(result, fmt.Sprintf("%s at %s:%d", frame.Function, frame.File, frame.Line))
		if !more {
			break
		}
	}

	return result
}
