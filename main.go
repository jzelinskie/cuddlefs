package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

func main() {
	kubeconfig := flag.String("kubeconfig", filepath.Join(os.ExpandEnv("$HOME"), ".kube", "config"), "path to kubeconfig")
	mountpoint := flag.String("mountpoint", "./kfs", "path where the filesystem will be mounted")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	c, err := fuse.Mount(
		*mountpoint,
		fuse.FSName("Kubernetes"),
		fuse.Subtype("k8sfs"),
		fuse.LocalVolume(),
		fuse.VolumeName("Kubernetes"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	err = fs.Serve(c, FS{clientset})
	if err != nil {
		log.Fatal(err)
	}

	<-c.Ready
	if err := c.MountError; err != nil {
		log.Fatal(err)
	}
}

type Resource struct {
	clientset *kubernetes.Clientset
	namespace string
	resource  string
	name      string
}

func (r Resource) getYAML() []byte {
	// TODO(jzelinskie): ya know, do more than just pods
	pod, _ := r.clientset.CoreV1().Pods(r.namespace).Get(r.name, metav1.GetOptions{})
	podYAML, _ := yaml.Marshal(pod)
	return podYAML
}

func (r Resource) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Size = uint64(len(r.getYAML()))
	return nil
}

func (r Resource) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	contents := r.getYAML()
	resp.Data = contents[int(req.Offset) : int(req.Offset)+int(req.Size)]
	return nil
}

type Pods struct {
	clientset *kubernetes.Clientset
	namespace string
}

func (Pods) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (p Pods) podNames() []string {
	podList, err := p.clientset.CoreV1().Pods(p.namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	names := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		names = append(names, pod.Name)
	}
	return names
}

func (p Pods) inPodNames(name string) bool {
	for _, podName := range p.podNames() {
		if name == podName {
			return true
		}
	}
	return false
}

func (p Pods) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	podNames := p.podNames()
	entries := make([]fuse.Dirent, 0, len(podNames))
	for _, name := range podNames {
		entries = append(entries, fuse.Dirent{Name: name, Type: fuse.DT_File})
	}
	return entries, nil
}

func (p Pods) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if p.inPodNames(name) {
		return Resource{p.clientset, p.namespace, "pod", name}, nil
	}
	return nil, fuse.ENOENT
}

type FS struct {
	clientset *kubernetes.Clientset
}

func (fs FS) Root() (fs.Node, error) {
	return Namespaces{fs.clientset}, nil
}

type Namespaces struct {
	clientset *kubernetes.Clientset
}

func (n Namespaces) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (n Namespaces) nsNames() []string {
	nsList, err := n.clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}

	return names
}

func (n Namespaces) inNamespaceNames(name string) bool {
	for _, nsName := range n.nsNames() {
		if name == nsName {
			return true
		}
	}
	return false
}

func (n Namespaces) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	nsNames := n.nsNames()
	entries := make([]fuse.Dirent, 0, len(nsNames))
	for _, name := range nsNames {
		entries = append(entries, fuse.Dirent{Name: name, Type: fuse.DT_Dir})
	}
	return entries, nil
}

func (ns Namespaces) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if ns.inNamespaceNames(name) {
		return Resources{ns.clientset, name}, nil
	}
	return nil, fuse.ENOENT
}

type Resources struct {
	clientset *kubernetes.Clientset
	namespace string
}

func (d Resources) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

func (r Resources) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return []fuse.Dirent{{Name: "pods", Type: fuse.DT_Dir}}, nil
}

func (r Resources) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if name == "pods" {
		return Pods{r.clientset, r.namespace}, nil
	}

	return nil, fuse.ENOENT
}
