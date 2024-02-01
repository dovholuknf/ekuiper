// Copyright 2023 EMQ Technologies Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"fmt"
	"io"
	"os"
	"strings"

	filename "github.com/keepeye/logrus-filename"
	"github.com/sirupsen/logrus"
)

var (
	Log       *logrus.Logger
	LogFile   *os.File
	IsTesting bool
)

const KuiperSyslogKey = "KuiperSyslogKey"

func init() {
	InitLogger()
}

func InitLogger() {
	if LogFile != nil {
		return
	}
	Log = logrus.New()
	filenameHook := filename.NewHook()
	filenameHook.Field = "file"
	Log.AddHook(filenameHook)

	Log.SetFormatter(&logrus.TextFormatter{
		DisableColors: true,
		FullTimestamp: true,
	})
	Log.Debugf("init with args %s", os.Args)
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			IsTesting = true
			break
		}
	}

	hook := &LogrusAdaptor{
		lc: Log,
	}
	logrus.StandardLogger().SetLevel(logrus.PanicLevel)
	logrus.AddHook(hook)
}

func CloseLogger() {
	if LogFile != nil {
		LogFile.Close()
	}
}

type LogrusAdaptor struct {
	lc *logrus.Logger
}

func (f *LogrusAdaptor) Format(entry *logrus.Entry) ([]byte, error) {
	// Implement your custom formatting logic here
	return []byte(fmt.Sprintf("[%s] %s\n", entry.Level, entry.Message)), nil
}

func (f *LogrusAdaptor) Levels() []logrus.Level {
	return logrus.AllLevels
}
func (f *LogrusAdaptor) Fire(e *logrus.Entry) error {
	switch e.Level {
	case logrus.DebugLevel:
		f.lc.Debug(e.Message)
	case logrus.InfoLevel:
		f.lc.Info(e.Message)
	case logrus.WarnLevel:
		f.lc.Warn(e.Message)
	case logrus.ErrorLevel:
		f.lc.Error(e.Message)
	case logrus.FatalLevel:
		f.lc.Error(e.Message)
	case logrus.PanicLevel:
		f.lc.Error(e.Message)
	}

	return nil
}

func AdaptLogrusBasedLogging(l *logrus.Logger) {

	// Create a new logger instance
	hook := &LogrusAdaptor{
		lc: l,
	}
	logrus.AddHook(hook)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableColors: true,
	})
	logrus.SetOutput(io.Discard)
}
