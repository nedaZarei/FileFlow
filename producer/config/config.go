package config

import "github.com/spf13/viper"

type Config struct {
	Server Server `yaml:"server"`
	Kafka  Kafka  `yaml:"kafka"`
}

type Server struct {
	Port string `yaml:"port"`
}

type Kafka struct {
	Broker string `yaml:"broker"`
	Topic  string `yaml:"topic"`
}

func InitConfig(filename string) (*Config, error) {
	v := viper.New() //will be used to manage configuration settings
	v.SetConfigFile(filename)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil { //parses the configuration data to Config struct
		return nil, err
	}
	return cfg, nil
}
