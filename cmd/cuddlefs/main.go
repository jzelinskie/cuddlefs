package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"k8s.io/client-go/tools/clientcmd"

	cuddlefs "github.com/jzelinskie/cuddlefs/pkg/fs"
	"github.com/jzelinskie/cuddlefs/pkg/kubeutil"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "cuddlefs [flags]",
		Short: "k8s filesystem",
		Long:  "cuddlefs is a userspace filesystem for Kubernetes",
		RunE:  rootRunFunc,
	}

	rootCmd.Flags().String("mountName", "current-context", "path where the filesystem will be mounted")
	rootCmd.Flags().String("volumeName", "current-context", "volume name for the mounted filesystem")
	rootCmd.Flags().String("kubeconfig", filepath.Join(os.ExpandEnv("$HOME"), ".kube", "config"), "path to kubeconfig")
	rootCmd.Flags().Bool("debug", false, "enable debug logging")
	rootCmd.Flags().Bool("fusedebug", false, "enable fuse debug logging")

	rootCmd.Execute()
}

func mustGetString(logger *zap.Logger, cmd *cobra.Command, flag string) string {
	value, err := cmd.Flags().GetString(flag)
	if err != nil {
		panic(err)
	}
	logger.Debug("parsed flag",
		zap.String("name", flag),
		zap.String("value", value),
	)
	return value
}

func mustGetBool(cmd *cobra.Command, flag string) bool {
	value, err := cmd.Flags().GetBool(flag)
	if err != nil {
		panic(err)
	}
	return value
}

func rootRunFunc(cmd *cobra.Command, args []string) error {
	var logger *zap.Logger
	var err error
	if mustGetBool(cmd, "debug") {
		logger, err = zap.NewDevelopment()

		if mustGetBool(cmd, "fusedebug") {
			logger.Debug("enabled fuse logging")
			fuse.Debug = func(msg interface{}) {
				logger.Debug("fuse", zap.Reflect("msg", msg))
			}
		}
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		return err
	}
	defer logger.Sync()

	kubeconfigPath := mustGetString(logger, cmd, "kubeconfig")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return err
	}
	logger.Debug("parsed kubeconfig",
		zap.String("host", config.Host),
	)

	cfs, err := cuddlefs.New(logger, config)
	if err != nil {
		return err
	}

	currentContext, err := kubeutil.CurrentContextName(kubeconfigPath)
	if err != nil {
		logger.Warn("failed to parse context name from kubeconfig", zap.Error(err))
		return err
	}
	logger.Debug("parsed current-context", zap.String("value", currentContext))

	mountName := stringz.Default(mustGetString(logger, cmd, "mountName"), currentContext, "", "current-context")
	volumeName := stringz.Default(mustGetString(logger, cmd, "volumeName"), currentContext, "", "current-context")

	c, err := fuse.Mount(
		mountName,
		fuse.FSName("cuddlefs"),
		fuse.Subtype("cuddlefs"),
		fuse.LocalVolume(),
		fuse.VolumeName(volumeName),
	)
	if err != nil {
		return err
	}
	defer c.Close()

	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for range quit {
			logger.Info("received interrupt")
			fuse.Unmount(mountName)
			if err != nil {
				logger.Error("failure unmounting", zap.Error(err))
			}
		}
	}()

	err = fs.Serve(c, cfs)
	if err != nil {
		return err
	}

	<-c.Ready
	if err := c.MountError; err != nil {
		return err
	}

	return nil
}
