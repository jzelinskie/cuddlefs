package main

import (
	"os"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/spf13/cobra"
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

	rootCmd.Execute()
}

func mustGetString(cmd *cobra.Command, flag string) string {
	value, err := cmd.Flags().GetString(flag)
	if err != nil {
		panic(err)
	}
	return value
}

func rootRunFunc(cmd *cobra.Command, args []string) error {
	config, err := clientcmd.BuildConfigFromFlags("", mustGetString(cmd, "kubeconfig"))
	if err != nil {
		return err
	}

	cfs, err := cuddlefs.New(config)
	if err != nil {
		return err
	}

	c, err := fuse.Mount(
		mustGetString(cmd, "mount"),
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
