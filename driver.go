package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types/plugins/logdriver"
	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	protoio "github.com/gogo/protobuf/io"
	"github.com/pkg/errors"
	"github.com/tonistiigi/fifo"
)

type driver struct {
	mu     sync.Mutex
	logs   map[string]*logPair
	idx    map[string]*logPair
	logger logger.Logger
}

type logPair struct {
	l               logger.Logger
	stream          io.ReadCloser
	info            logger.Info
	unsafeAliveFlag bool
}

func newDriver() *driver {
	return &driver{
		logs: make(map[string]*logPair),
		idx:  make(map[string]*logPair),
	}
}

const logBasePathLabelName = "LogRootPath"
const partitionIdLabelName = "PartitionId";
const instanceIdLabelName = "ServicePackageActivationId";
const codePackageLabelName = "CodePackageName";
const codePackageInstanceLabelName = "CodePackageInstance";
const digestedApplicationLabelName = "DigestedApplicationName";

const logBasePathDefault = "/mnt/logs"

const jsonfileLogOptionMaxFileSizeName = "max-size";
const jsonfileLogOptionMaxFileCountName = "max-file";

const jsonfileLogOptionMaxFileSizeDefault = "1m";
const jsonfileLogOptionMaxFileCountDefault = "50";

func (d *driver) StartLogging(file string, logCtx logger.Info) error {
	d.mu.Lock()
	if _, exists := d.logs[file]; exists {
		d.mu.Unlock()
		logrus.WithField("id", logCtx.ContainerID).WithField("file", file).WithField("logpath", logCtx.LogPath).Debugf(fmt.Sprintf("logger for %s already exists", file))

		return fmt.Errorf("logger for %q already exists", file)
	}
	d.mu.Unlock()

	var basePath = strings.TrimSpace(logCtx.ContainerLabels[logBasePathLabelName])

	// ensure the full path provided by labels exists.
	// While we are sure that the path mounted to the log driver exists, the base path can
	// contain sub directories below that path so we have to check that they exist
	if (len(basePath) > 0) {
		if fileInfo, err := os.Stat(logCtx.ContainerLabels[logBasePathLabelName]); err != nil {
			logrus.WithField("logpath", logCtx.ContainerLabels[logBasePathLabelName]).WithField("error", err).Errorf("Error with provided base path.")
			return err
		} else if !fileInfo.Mode().IsDir() {
			logrus.WithField("logpath", logCtx.ContainerLabels[logBasePathLabelName]).Errorf("Error with provided base path. It is not a path.")
			return errors.New("Provided log path is not a directory.")
		}
	} else {
		logrus.Warningf("Log driver base path label is not set on the container. Will use the base path of /mnt/logs")
		basePath = logBasePathDefault
	}

	// logs are written to /mnt/logs/$ApplicationName/$PartitionId/$InstanceId/$CodePackageName/$number/application.log
	logCtx.LogPath = filepath.Join(
		basePath,
		logCtx.ContainerLabels[digestedApplicationLabelName],
		logCtx.ContainerLabels[partitionIdLabelName],
		logCtx.ContainerLabels[instanceIdLabelName],
		logCtx.ContainerLabels[codePackageLabelName],
		logCtx.ContainerLabels[codePackageInstanceLabelName],
		"application.log")

	if err := os.MkdirAll(filepath.Dir(logCtx.LogPath), 0755); err != nil {
		return errors.Wrap(err, "error setting up logger dir")
	}

	// set default file options to 50 files of 1m each if no value is set.
	if _, exists := logCtx.Config[jsonfileLogOptionMaxFileSizeName]; !exists {
		logCtx.Config[jsonfileLogOptionMaxFileSizeName] = jsonfileLogOptionMaxFileSizeDefault
	}

	if _, exists := logCtx.Config[jsonfileLogOptionMaxFileCountName]; !exists {
		logCtx.Config[jsonfileLogOptionMaxFileSizeName] = jsonfileLogOptionMaxFileCountDefault
	}

	l, err := jsonfilelog.New(logCtx)
	if err != nil {
		return errors.Wrap(err, "error creating jsonfile logger")
	}

	logrus.WithField("id", logCtx.ContainerID).WithField("file", file).WithField("logpath", logCtx.LogPath).Debugf("Start logging")
	f, err := fifo.OpenFifo(context.Background(), file, syscall.O_RDONLY, 0700)
	if err != nil {
		return errors.Wrapf(err, "error opening logger fifo: %q", file)
	}

	d.mu.Lock()
	lf := &logPair{l, f, logCtx, true}
	d.logs[file] = lf
	d.idx[logCtx.ContainerID] = lf
	d.mu.Unlock()

	go consumeLog(lf)
	return nil
}

func (d *driver) StopLogging(file string) error {
	logrus.WithField("file", file).Debugf(fmt.Sprintf("Stop logging %s", file))
	d.mu.Lock()
	lf, ok := d.logs[file]
	if ok {
		lf.unsafeAliveFlag = false
		lf.stream.Close()
		delete(d.logs, file)
		logrus.WithField("file", file).Debugf(fmt.Sprintf("Logging stream closed for %s", file))
	} else {
		logrus.WithField("file", file).Errorf(fmt.Sprintf("Logging stream did not closed for %s. No such file was found.", file))
	}
	d.mu.Unlock()
	return nil
}

func consumeLog(lf *logPair) {
	dec := protoio.NewUint32DelimitedReader(lf.stream, binary.BigEndian, 1e6)
	defer dec.Close()
	var buf logdriver.LogEntry
	for {
		if !lf.unsafeAliveFlag {
			logrus.WithField("id", lf.info.ContainerID).Infof("shutting down log logger due to alive flag")
			lf.stream.Close()
			logrus.WithField("id", lf.info.ContainerID).Infof("log logger shut downed due to alive flag")
			return
		}

		if err := dec.ReadMsg(&buf); err != nil {
			if err == io.EOF {
				logrus.WithField("id", lf.info.ContainerID).WithError(err).Debugf("shutting down log logger")
				lf.stream.Close()
				return
			}
			dec = protoio.NewUint32DelimitedReader(lf.stream, binary.BigEndian, 1e6)
		}
		var msg logger.Message
		msg.Line = buf.Line
		msg.Source = buf.Source
		msg.PLogMetaData = nil
		msg.Timestamp = time.Unix(0, buf.TimeNano)

		if err := lf.l.Log(&msg); err != nil {
			logrus.WithField("id", lf.info.ContainerID).WithError(err).WithField("message", msg).Error("error writing log message")
			continue
		}

		buf.Reset()
	}
}

func (d *driver) ReadLogs(info logger.Info, config logger.ReadConfig) (io.ReadCloser, error) {
	d.mu.Lock()
	lf, exists := d.idx[info.ContainerID]
	d.mu.Unlock()
	if !exists {
		return nil, fmt.Errorf("logger does not exist for %s", info.ContainerID)
	}

	r, w := io.Pipe()
	lr, ok := lf.l.(logger.LogReader)
	if !ok {
		return nil, fmt.Errorf("logger does not support reading")
	}

	go func() {
		watcher := lr.ReadLogs(config)

		enc := protoio.NewUint32DelimitedWriter(w, binary.BigEndian)
		defer enc.Close()
		defer watcher.Close()

		var buf logdriver.LogEntry
		for {
			select {
			case msg, ok := <-watcher.Msg:
				if !ok {
					w.Close()
					return
				}

				buf.Line = msg.Line
				buf.Partial = (msg.PLogMetaData != nil)
				buf.TimeNano = msg.Timestamp.UnixNano()
				buf.Source = msg.Source

				if err := enc.WriteMsg(&buf); err != nil {
					w.CloseWithError(err)
					return
				}
			case err := <-watcher.Err:
				w.CloseWithError(err)
				return
			}

			buf.Reset()
		}
	}()

	return r, nil
}
