package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/sdk"
)

const pluginSocketAddress = "/run/docker/plugins/jsonfile.sock"

var logLevels = map[string]logrus.Level{
	"debug": logrus.DebugLevel,
	"info":  logrus.InfoLevel,
	"warn":  logrus.WarnLevel,
	"error": logrus.ErrorLevel,
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.Infof("Logger started")

	h := sdk.NewHandler(`{"Implements": ["LoggingDriver"]}`)
	handlers(&h, newDriver())

	logrus.Infof("listening to plugin socket at %s", pluginSocketAddress)
	if err := h.ServeUnix(pluginSocketAddress, 0); err != nil {
		panic(err)
	}
}
