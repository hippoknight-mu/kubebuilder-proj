package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	// "griffin/k8s-cluster-mgmt/pkg/api/processdump/v1beta1"
	// pdclientset "griffin/k8s-cluster-mgmt/pkg/generated/clientset/versioned"
	// pdclient "griffin/k8s-cluster-mgmt/pkg/generated/clientset/versioned/typed/processdump/v1beta1"
	v1beta1 "pd-proj/api/v1beta1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	// utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"

	// ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

// var (
// 	scheme = runtime.NewScheme()
// )

// func init() {
// 	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
// 	utilruntime.Must(v1beta1.AddToScheme(scheme))
// }

// // var SchemeGroupVersion = schema.GroupVersion{Group: CRDGroup, Version: CRDVersion}
// var SchemeGroupVersion = v1beta1.GroupVersion

// func addKnownTypes(scheme *runtime.Scheme) error {
// 	scheme.AddKnownTypes(SchemeGroupVersion,
// 		&v1beta1.ProcessDump{},
// 		&v1beta1.ProcessDumpList{},
// 	)
// 	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
// 	return nil
// }

func main() {
	// creates the in-cluster config
	// config, err := rest.InClusterConfig()
	// scheme := runtime.NewScheme()
	// addKnownTypes(scheme)
	// SchemeBuilder := runtime.NewSchemeBuilder(addKnownTypes)
	// if err := SchemeBuilder.AddToScheme(scheme); err != nil {
	// 	klog.Info(err.Error())
	// }
	// scheme.AddKnownTypes(SchemeGroupVersion,
	// 	&v1beta1.ProcessDump{},
	// 	&v1beta1.ProcessDumpList{},
	// )	
	// if err := clientgoscheme.AddToScheme(scheme); err != nil {
	// 	klog.Info(err.Error())
	// }
	

	// kubeconfig := "/home/hippo/.kube/config"
	kubeconfig := "/home/muqiao/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	checkErr(err, "")
	config.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"} // this is required when using kubectl/cp, don't know why not in exec
	if config.APIPath == "" {
		config.APIPath = "/api"
	}
	if config.NegotiatedSerializer == nil {
		config.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()
	}
	if len(config.UserAgent) == 0 {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	clientset, err := kubernetes.NewForConfig(config)
	checkErr(err, "")

	pdconfig := ctrlconfig.GetConfigOrDie()
	pdclient, err := client.New(pdconfig, client.Options{})
	checkErr(err, "")

	opts := &ExecOptions{
		clientset:  clientset,
		restConfig: config,
		pdclient:   pdclient,
	}

	opts.parseEnv()

// ####################################################################
	v1beta1.AddToScheme(pdclient.Scheme())
// 	var pd v1beta1.ProcessDump
// 	pdKey := types.NamespacedName {Namespace: opts.namespace, Name: opts.procdumpName}
// 	if err = pdclient.Get(context.Background(), pdKey, &pd); err != nil {
// 		klog.Error(err.Error())
// 	}

// 	pd.Status.WorkerPodName = "updated-Pod-Name"
// 	if err = pdclient.Status().Update(context.Background(), &pd); err != nil {
// 		klog.Error(err.Error())
// 	}
// 	if err = pdclient.Get(context.Background(), pdKey, &pd); err != nil {
// 		klog.Error(err.Error())
// 	}
// 	klog.Infof("$$$$$$$$$$$$$$NEWNAME %s $$$$$$$$$$$$", pd.Status.WorkerPodName)
// 	klog.Infof("$$$$$$$$$$$$$$ %+v $$$$$$$$$$$$", pd)
	
// 	return 
// // ####################################################################

	// dump steps:
	// 0. validate pod, detect os (sh or powershell)
	// 1. output process list, `ps` or `Get-Process`
	// 2. create dump, only support windows for now

	// Step 0
	err = opts.validatePod()
	checkErr(err, "")

	// Step 1
	opts.getProcessList()

	// Step 2
	if opts.podOS == "windows" {
		if opts.procID == "0" && opts.procName == "" {
			fmt.Println("Please re-create a processdump resource and specify the process-id")
			condition := v1beta1.ProcessDumpCondition{
				Reason:             "Process not specified",
				Type:               v1beta1.WatsonSucceeded,
				Status:             metav1.ConditionFalse,
				Message:            "Please re-create a processdump resource and specify the process-id",
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
			opts.updateStatus(condition)
			// block
			<-(chan int)(nil)
		} else {
			opts.watsonDump()
		}
	} else {
		// TODO, linux dump
		fmt.Println("Dump in linux container not supported yet.")
	}

	fmt.Println("WorkerPod finished.")

	// block
	<-(chan int)(nil)
}

func (o *ExecOptions) parseEnv() {
	o.namespace = os.Getenv("NAMESPACE")
	o.podName = os.Getenv("POD_NAME")
	if o.podName == "" {
		panic("pod name cannot be empty")
	}
	o.containerName = os.Getenv("CONTAINER_NAME")
	o.procName = os.Getenv("PROCESS_NAME")
	o.procID = os.Getenv("PROCESS_ID")
	o.procdumpName = os.Getenv("PROCESS_DUMP_NAME")
}

func (o *ExecOptions) validatePod() error {
	// get pod
	targetpod, err := o.clientset.CoreV1().Pods(o.namespace).Get(context.TODO(), o.podName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		fmt.Printf("Pod %v not found in Namespace %v\n", o.podName, o.namespace)
		condition := v1beta1.ProcessDumpCondition{
			Reason:             "Target pod not found",
			Type:               v1beta1.ProcessListRetrieved,
			Status:             metav1.ConditionFalse,
			Message:            fmt.Sprintf("Pod %v not found in Namespace %v", o.podName, o.namespace),
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		o.updateStatus(condition)
		// block, workerpod finish
		<-(chan int)(nil)
	} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		fmt.Printf("Error getting Pod %v\n", statusError.ErrStatus.Message)
	} else if err != nil {
		panic(err.Error())
	} else {
		fmt.Printf("Found pod %v in Namespace %v\n", o.podName, o.namespace)
	}
	if o.containerName == "" {
		o.containerName = targetpod.Spec.Containers[0].Name
	} else {
		// TODO, validate given container exists in pod
	}

	// get OS
	var found bool
	if o.podOS, found = targetpod.Spec.NodeSelector["beta.kubernetes.io/os"]; !found {
		if o.podOS, found = targetpod.Spec.NodeSelector["kubernetes.io/os"]; !found {
			fmt.Printf("found no nodeSelector in targetpod(%v), assume is linux\n", o.podName)
			o.podOS = "linux"
			return nil
		}
	}
	fmt.Println("Pod OS: " + o.podOS)
	return nil
}

func (o *ExecOptions) getProcessList() {
	fmt.Printf("Getting process list of pod(%v)/container(%v)...\n\n", o.podName, o.containerName)
	var err error
	var b bytes.Buffer
	var msg string

	if o.podOS == "linux" {
		cmd := []string{
			"sh",
			"-c",
			"ps",
		}
		err = o.execCmd(cmd, nil, os.Stdout, os.Stderr)
	} else {
		cmd := []string{
			"powershell.exe",
			`Get-process|Select-object Id, ProcessName`,
		}
		_ = o.execCmd(cmd, nil, &b, os.Stderr)
		msg = b.String()
		// klog.Info(msg)
	}
	checkErr(err, "Retrieving process list failed")

	// Update processdump status, append condition PrcessListRetrieved
	condition := v1beta1.ProcessDumpCondition{
		Reason:             "ProcessListRetrieved",
		Type:               v1beta1.ProcessListRetrieved,
		Status:             metav1.ConditionTrue,
		Message:            "==== Process List in Container from kuberbuilder ====\n" + msg,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
	o.updateStatus(condition)
}

func (o *ExecOptions) watsonDump() {
	err := o.CopyToPod("/run-dump.ps1", "/run-dump.ps1")
	checkErr(err, "")

	scriptparam := "C:\\run-dump.ps1 "
	if o.procID != "0" {
		scriptparam += "-ProcID " + o.procID
	} else {
		scriptparam += "-ProcName " + o.procName
	}
	cmd := []string{
		"powershell.exe",
		scriptparam,
	}

	klog.Infoln("Start dump ... ")
	var bf bytes.Buffer
	// err = o.execCmd(cmd, nil, &bf, nil)
	err = o.execCmd(cmd, nil, &bf, &bf)
	checkErr(err, "Watson start failed")
	klog.Info(bf.String())

	var b bytes.Buffer
	for i := 0; i < 20; i++ {
		cmd := []string{
			"powershell.exe",
			"cat c:\\log.txt",
		}
		o.execCmd(cmd, nil, &b, &bf)
		if b.Len() > 0 && strings.Contains(b.String(), "http") {
			url := b.String()
			klog.Info(url)
			fmt.Printf("Dump uploaded to: %s", url)
			// Update processdump status, append condition PrcessListRetrieved
			condition := v1beta1.ProcessDumpCondition{
				Reason:             "DumpUploaded",
				Type:               v1beta1.WatsonSucceeded,
				Status:             metav1.ConditionTrue,
				Message:            "==== Dump uploaded to ====\n" + url,
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
			o.updateStatus(condition)
			return
		} else {
			time.Sleep(2 * time.Second)
		}
	}

	// Dump failed
	condition := v1beta1.ProcessDumpCondition{
		Reason:             "JitWatson failed",
		Type:               v1beta1.WatsonSucceeded,
		Status:             metav1.ConditionFalse,
		Message:            "JitWatson failed, please verify the input processID and re-create processdump resource",
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
	o.updateStatus(condition)
}

func (o *ExecOptions) execCmd(cmd []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	req := o.clientset.CoreV1().RESTClient().Post().Resource("pods").Name(o.podName).
		Namespace(o.namespace).SubResource("exec")

	option := &corev1.PodExecOptions{
		Container: o.containerName,
		Command:   cmd,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}
	req.VersionedParams(
		option,
		clientgoscheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(o.restConfig, "POST", req.URL())

	if err != nil {
		return err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
	if err != nil {
		return err
	}
	time.Sleep(time.Second)
	fmt.Println()

	return nil
}

func (o *ExecOptions) updateStatus(condition v1beta1.ProcessDumpCondition) {
	var err error
	var pd v1beta1.ProcessDump
	// o.pdclient.Scheme().AddKnownTypes(v1beta1.GroupVersion, &v1beta1.ProcessDump{}, &v1beta1.ProcessDumpList{})
	pdKey := types.NamespacedName{Namespace: o.namespace, Name: o.procdumpName}
	if err = o.pdclient.Get(context.Background(), pdKey, &pd); err != nil {
		klog.Info("Updatestatus failed")
		klog.Error(err.Error())
	}
	pd.Status.Conditions = append(pd.Status.Conditions, condition)
	klog.Infof("######### %+v", pd)
	
	if err = o.pdclient.Status().Update(context.Background(), &pd); err != nil {
		// klog.Infof("%+v", o.pdclient.Scheme())
		klog.Error(err.Error())
	}
	checkErr(err, "Updatestatus failed")
}

type ExecOptions struct {
	clientset kubernetes.Interface
	// pdclient      pdclient.ProcessDumpsGetter
	pdclient      client.Client
	restConfig    *rest.Config
	podName       string
	containerName string
	namespace     string
	podOS         string
	procName      string
	procID        string
	procdumpName  string
}

func checkErr(err error, msg string) {
	if err != nil {
		if msg != "" {
			fmt.Println(msg)
		}
		panic(err.Error())
	}
}
