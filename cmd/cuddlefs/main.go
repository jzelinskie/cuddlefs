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
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "cuddlefs [flags]",
		Short: "k8s filesystem",
		Long:  "cuddlefs is a userspace filesystem for Kubernetes",
		RunE:  rootRunFunc,
	}

	rootCmd.Flags().String("mount", "./cluster", "path where the filesystem will be mounted")
	rootCmd.Flags().String("kubeconfig", filepath.Join(os.ExpandEnv("$HOME"), ".kube", "config"), "path to kubeconfig")
	rootCmd.Flags().Bool("debug", false, "enable debug logging")

	rootCmd.Execute()
}

func mustGetString(cmd *cobra.Command, flag string) string {
	value, err := cmd.Flags().GetString(flag)
	if err != nil {
		panic(err)
	}
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

	kubeconfigPath := mustGetString(cmd, "kubeconfig")
	logger.Debug("parsed env var",
		zap.String("name", "kubeconfig"),
		zap.String("value", kubeconfigPath),
	)

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

	mountPath := mustGetString(cmd, "mount")
	logger.Debug("parsed env var",
		zap.String("name", "mount"),
		zap.String("value", mountPath),
	)

	c, err := fuse.Mount(
		mountPath,
		fuse.FSName("cuddlefs"),
		fuse.Subtype("cuddlefs"),
		fuse.LocalVolume(),
		fuse.VolumeName("Kubernetes"),
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
