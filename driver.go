package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func (d *driver) StartLogging(file string, logCtx logger.Info) error {
	d.mu.Lock()
	if _, exists := d.logs[file]; exists {
		d.mu.Unlock()
		logrus.WithField("id", logCtx.ContainerID).WithField("file", file).WithField("logpath", logCtx.LogPath).Debugf(fmt.Sprintf("logger for %s already exists", file))

		return fmt.Errorf("logger for %q already exists", file)
	}
	d.mu.Unlock()

	// logs are written to /var/log/docker/$ContainerId/application.log
	// rotated to /var/log/docker/$ContainerId/application.x.log
	// example:
	// logs written to /var/log/docker/abc/application.log
	// log rotated to  /var/log/docker/abc/application.1.log
	logCtx.LogPath = filepath.Join("/var/log/docker", logCtx.ContainerID, "application.log")

	if err := os.MkdirAll(filepath.Dir(logCtx.LogPath), 0755); err != nil {
		return errors.Wrap(err, "error setting up logger dir")
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
			lf.steam.Close()
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
		msg.Partial = buf.Partial
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
				buf.Partial = msg.Partial
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
