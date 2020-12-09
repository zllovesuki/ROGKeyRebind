package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zllovesuki/G14Manager/box"
	"github.com/zllovesuki/G14Manager/controller"
	"github.com/zllovesuki/G14Manager/util"

	"cirello.io/oversight"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Compile time injected variables
var (
	Version = "dev"
	IsDebug = "yes"
)

var defaultCommandWithArgs = "Taskmgr.exe"

func main() {

	if IsDebug == "no" {
		log.SetOutput(&lumberjack.Logger{
			Filename:   `C:\Logs\G14Manager.log`,
			MaxSize:    5,
			MaxBackups: 3,
			MaxAge:     7,
			Compress:   true,
		})
	}

	var rogRemap util.ArrayFlags
	flag.Var(&rogRemap, "rog", "customize ROG key behavior when pressed multiple times")

	var enableRemap = flag.Bool("remap", false, "enable remapping Fn+Left/Right to PgUp/PgDown")
	var enableAutoThermal = flag.Bool("autoThermal", false, "enable automatic thermal profile switching on power source change")

	flag.Parse()

	log.Printf("G14Manager version: %s\n", Version)
	log.Printf("Remapping enabled: %v\n", *enableRemap)
	log.Printf("Automatic Thermal Profile Switching enabled: %v\n", *enableAutoThermal)

	var logoPath string
	logoPng := box.Get("/Logo.png")
	if logoPng != nil {
		logoFile, err := ioutil.TempFile(os.TempDir(), "G14Manager-")
		if err != nil {
			log.Fatal("[supervisor] Cannot create temporary file for logo", err)
		}
		defer func() {
			time.Sleep(time.Second)
			os.Remove(logoFile.Name())
		}()

		if _, err = logoFile.Write(logoPng); err != nil {
			log.Fatal("[supervisor] Failed to write to temporary file for logo", err)
		}

		if err := logoFile.Close(); err != nil {
			log.Fatal(err)
		}

		logoPath = logoFile.Name()
		log.Printf("[supervisor] Logo extracted to %s\n", logoPath)
	}

	controllerConfig := controller.RunConfig{
		LogoPath: logoPath,
		RogRemap: rogRemap,
		EnabledFeatures: controller.Features{
			FnRemap:            *enableRemap,
			AutoThermalProfile: *enableAutoThermal,
		},
		DryRun: os.Getenv("DRY_RUN") != "",
	}

	supervisor := oversight.New(
		oversight.WithRestartStrategy(oversight.OneForOne()),
		oversight.Process(oversight.ChildProcessSpecification{
			Name: "Controller",
			Start: func(ctx context.Context) error {
				control, err := controller.New(controllerConfig)
				if err != nil {
					return err
				}
				return control.Run(ctx)
			},
			Restart: func(err error) bool {
				if err == nil {
					return false
				}
				log.Println("[supervisor] controller returned an error:")
				log.Printf("%+v\n", err)
				util.SendToastNotification("G14Manager Supervisor", util.Notification{
					Title:   "G14Manager will be restarted",
					Message: fmt.Sprintf("An error has occurred: %s", err),
				})
				return true
			},
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())

	sigc := make(chan os.Signal, 1)
	signal.Notify(
		sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	go func() {
		log.Println("[supervisor] Monitoring controller")
		if err := supervisor.Start(ctx); err != nil {
			util.SendToastNotification("G14Manager Supervisor", util.Notification{
				Title:   "G14Manager cannot be started",
				Message: fmt.Sprintf("Error: %v", err),
			})
			log.Fatalf("[supervisor] controller start error: %v\n", err)
		}
	}()

	srv := &http.Server{Addr: "127.0.0.1:9969"}
	go func() {
		log.Println("[supervisor] pprof at 127.0.0.1:9969/debug/pprof")
		log.Println(srv.ListenAndServe())
	}()

	<-sigc

	srv.Shutdown(context.TODO())
	cancel()
	time.Sleep(time.Second) // 1 second for grace period
}