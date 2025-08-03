package main

import (
	"os"

	"github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd"
	"github.com/sirupsen/logrus"
)

func main() {
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	// LOG_LEVEL not set, let's default to info
	if !ok {
		lvl = "info"
	}
	// parse string, this is built-in feature of logrus
	ll, err := logrus.ParseLevel(lvl)
	if err != nil {
		ll = logrus.DebugLevel
	}
	// set global log level
	logrus.SetLevel(ll)
	if err := cmd.NewCmdFabricX().Execute(); err != nil {
		os.Exit(1)
	}
}
