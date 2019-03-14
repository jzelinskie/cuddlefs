package main

import (
	"os"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
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

	rootCmd.Flags().String("mount", "./cluster", "path where the filesystem will be mounted")
	rootCmd.Flags().String("volumeName", "current-context", "volume name for the mounted filesystem")
	rootCmd.Flags().String("kubeconfig", filepath.Join(os.ExpandEnv("$HOME"), ".kube", "config"), "path to kubeconfig")
	rootCmd.Flags().Bool("debug", false, "enable debug logging")

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

	volumeName := mustGetString(logger, cmd, "volumeName")
	if volumeName == "current-context" || volumeName == "" {
		volumeName, err = kubeutil.ContextName(kubeconfigPath)
		if err != nil {
			logger.Warn("failed to parse context name from kubeconfig", zap.Error(err))
			return err

		}
	}

	c, err := fuse.Mount(
		mustGetString(logger, cmd, "mount"),
		fuse.FSName("cuddlefs"),
		fuse.Subtype("cuddlefs"),
		fuse.LocalVolume(),
		fuse.VolumeName(volumeName),
	)
	if err != nil {
		return err
	}
	defer c.Close()

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
