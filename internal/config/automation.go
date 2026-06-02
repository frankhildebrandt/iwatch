package config

// AutomationConfig defines one runtime automation rule.
type AutomationConfig struct {
	ID      string           `json:"id"`
	Trigger string           `json:"trigger"`
	Regex   string           `json:"regex,omitempty"`
	Actions []AutomationAction `json:"actions,omitempty"`
}

