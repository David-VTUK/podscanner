package cmd

import (
	"context"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
)

type identifiedWorkload struct {
	containerName, namespace, image, pod string
}

// byNamespace implements sort.Interface based on the namespace field.
type byNamespace []identifiedWorkload

func (n byNamespace) Len() int           { return len(n) }
func (n byNamespace) Less(i, j int) bool { return n[i].namespace < n[j].namespace }
func (n byNamespace) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

// Channel buffer size
const (
	defaultKubeconfig = "~/.kube/config"
	burst             = 50
	qps               = 25
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "podscanner",
	Short: "Scans kubernetes pods for containers using the :latest tag, or no tag",
	Long: `A multi threaded tool that iterates through all pods in all namespaces (or filtered by --namespace)
and identifies Pods that contain containers where the image spec is :latest or missing a tag`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		//	cmd.Flags().Set("namespace", "default")
		namespaceFromFlag, err := cmd.Flags().GetString("namespace")
		kubeconfigFromFlag, err := cmd.Flags().GetString("kubeconfig")

		if kubeconfigFromFlag == "" {
			kubeconfigFromFlag, err = homeDir(defaultKubeconfig)
			if err != nil {
				handleError(err)
			}
		} else {
			kubeconfigFromFlag, err = homeDir(kubeconfigFromFlag)
			if err != nil {
				handleError(err)
			}
		}

		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigFromFlag)

		if err != nil {
			handleError(err)
		}

		// Increase the Burst and QOS values
		config.Burst = burst
		config.QPS = qps

		// Build client from  config
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			handleError(err)
		}

		ctx := context.Background()

		// Get the list of containers
		numberOfContainers, err := getTotalNumberOfContainers(ctx, clientset)
		if err != nil {
			handleError(err)
		}

		// Add a buffer of 10% - in case extra containers are spun whilst this app finishes
		numberOfContainers += numberOfContainers / 10

		// Grab the list of namespaces in the current context

		var listOfNamespaces []string

		if namespaceFromFlag != "" {
			fmt.Printf("--namespace flag used, only scanning %s\n", namespaceFromFlag)
			listOfNamespaces = append(listOfNamespaces, namespaceFromFlag)
		} else {
			fmt.Printf("--namespace flag not used, only scanning all namespces\n")
			listOfNamespaces, err = getNamespaces(ctx, clientset)
			if err != nil {
				handleError(err)
			}
		}

		// Put all threads in a waitgroup so the channel can be closed once all threads have finished
		var wg sync.WaitGroup
		// Match the number of waitgroups to the number of namespaces.
		// as each call to getPodsPerNamespace() will be in its own goroutine
		wg.Add(len(listOfNamespaces))

		// Retrieve responses from threaded calls
		messages := make(chan identifiedWorkload, numberOfContainers)

		//Iterate through the namespaces
		for _, namespace := range listOfNamespaces {
			//For each namespace, inspect the pods that reside it within a dedicated goroutine
			go func(n string) {
				if err := getPodsPerNamespace(ctx, n, clientset, messages); err != nil {
					handleError(err)
				}
				defer wg.Done()
			}(namespace)
		}
		// Wait for all goroutines to finish
		wg.Wait()

		// As this uses a buffered channel, we need to explicitly close
		close(messages)

		var listOfWorkloads []identifiedWorkload
		for element := range messages {
			listOfWorkloads = append(listOfWorkloads, element)
		}

		sort.Sort(byNamespace(listOfWorkloads))
		displayWorkloads(listOfWorkloads)

	},
}

func handleError(err error) {
	panic(err.Error())
}

func homeDir(filename string) (string, error) {
	// Replace homedir reference if required
	if strings.Contains(filename, "~/") {
		homedir, err := os.UserHomeDir()
		if err != nil {
			handleError(err)
		}
		filename = strings.Replace(filename, "~/", "", 1)
		filename = path.Join(homedir, filename)
	}

	// Check we can read the file
	if _, err := os.Stat(filename); err != nil {
		handleError(err)
		return "", err
	}
	return filename, nil
}

// Return the list of namespaces in the cluster
func getNamespaces(ctx context.Context, c *kubernetes.Clientset) ([]string, error) {
	var listOfNamespaces []string
	namespaces, err := c.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		handleError(err)
		return listOfNamespaces, err
	}

	for _, namespace := range namespaces.Items {
		listOfNamespaces = append(listOfNamespaces, namespace.Name)
	}
	return listOfNamespaces, nil
}

// Iterate through Namespace -> Pod -> Container and identify container images
// using either :latest or no tag
func getPodsPerNamespace(ctx context.Context, namespace string, clientSet *kubernetes.Clientset, c chan identifiedWorkload) error {
	pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		handleError(err)
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			if strings.Contains(container.Image, "latest") == true || strings.Contains(container.Image, ":") == false {
				cont := identifiedWorkload{
					containerName: container.Name,
					namespace:     namespace,
					image:         container.Image,
					pod:           pod.Name,
				}
				c <- cont
			}
		}
	}

	return nil
}

//Display the results
func displayWorkloads(w []identifiedWorkload) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Namespace", "Pod", "Container", "Image"})

	for _, container := range w {
		t.AppendRow(table.Row{
			container.namespace, container.pod, container.containerName, container.image,
		})
		t.AppendSeparator()
	}
	t.SetStyle(table.StyleLight)

	//render table
	t.Render()
}

//Get the list of pods in the cluster. This will determine the buffer size of the channel
func getTotalNumberOfContainers(ctx context.Context, clientSet *kubernetes.Clientset) (int, error) {

	numberOfContainers := 0
	pods, err := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, err
	}

	for _, pod := range pods.Items {
		numberOfContainers += len(pod.Spec.Containers)
	}
	return numberOfContainers, nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	//	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cobra-demo-app.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().StringP("namespace", "n", "", "Optional: specify namespace (if not defined, all namespaces will be scanned)")
	rootCmd.Flags().StringP("kubeconfig", "k", "", "Optional: specify kubeconfig (if not defined, defaults to ~/.kube/config")
}
