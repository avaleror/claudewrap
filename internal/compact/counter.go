// Package compact tracks how many times /compact has run in a session and
// emits warnings when repeated compaction starts to degrade response quality.
package compact

const (
	WarnThreshold    = 2 // compaction count at which quality starts degrading
	RestartThreshold = 3 // compaction count at which a session restart is recommended
)

type Warning int

const (
	WarnNone    Warning = iota
	WarnQuality         // 2+ compactions
	WarnRestart         // 3+ compactions
)

func GetWarning(count int) Warning {
	switch {
	case count >= RestartThreshold:
		return WarnRestart
	case count >= WarnThreshold:
		return WarnQuality
	default:
		return WarnNone
	}
}

func WarningText(count int) string {
	switch GetWarning(count) {
	case WarnQuality:
		return "Compacted 2x — quality degrading"
	case WarnRestart:
		return "Compacted 3x — consider restarting session"
	default:
		return ""
	}
}
