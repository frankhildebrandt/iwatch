package config

// AutomationAction defines one side effect executed when an automation triggers.
type AutomationAction struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

