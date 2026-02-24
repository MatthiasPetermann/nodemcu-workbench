package ui

type Mode int

const (
	ModeWorkbench Mode = iota
	ModeTerminal
	ModeMaintenance
)

func (m Mode) String() string {
	switch m {
	case ModeWorkbench:
		return "WORKBENCH"
	case ModeTerminal:
		return "TERMINAL"
	case ModeMaintenance:
		return "MAINTENANCE"
	default:
		return "UNKNOWN"
	}
}

type StatusKind int

const (
	StatusInfo StatusKind = iota
	StatusWarn
	StatusError
)

type StatusMsg struct {
	Kind StatusKind
	Text string
}

type PromptKind int

const (
	PromptRename PromptKind = iota
	PromptNewDir
	PromptConfirmDelete
	PromptNewFile
)

type PromptRequest struct {
	Active      bool
	Kind        PromptKind
	Label       string
	Placeholder string
	Initial     string
}

type PromptResultMsg struct {
	Kind     PromptKind
	Accepted bool
	Value    string
}
