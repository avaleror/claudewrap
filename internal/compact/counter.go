package compact

const (
	WarnThreshold    = 2
	RestartThreshold = 3
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
