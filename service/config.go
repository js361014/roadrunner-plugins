package service

import "time"

// Service represents particular service configuration
type Service struct {
	Command         string        `mapstructure:"command"`
	Output          string        `mapstructure:"log_output"`
	LineEnding      string        `mapstructure:"line_ending"`
	ErrColor        string        `mapstructure:"err_color"`
	RegularColor    string        `mapstructure:"color"`
	ProcessNum      int           `mapstructure:"process_num"`
	ExecTimeout     time.Duration `mapstructure:"exec_timeout"`
	RemainAfterExit bool          `mapstructure:"remain_after_exit"`
	RestartSec      uint64        `mapstructure:"restart_sec"`
}

// Config for the services
type Config struct {
	Services map[string]Service `mapstructure:"service"`
}

func (c *Config) InitDefault() {
	if len(c.Services) > 0 {
		for k, v := range c.Services {
			val := c.Services[k]
			val.LineEnding = "\n"
			c.Services[k] = val

			if v.ProcessNum == 0 {
				val := c.Services[k]
				val.ProcessNum = 1
				c.Services[k] = val
			}
			if v.RestartSec == 0 {
				val := c.Services[k]
				val.RestartSec = 30
				c.Services[k] = val
			}
		}
	}
}
