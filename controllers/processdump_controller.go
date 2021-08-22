package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	diagnosticsofficecomv1beta1 "pd-proj/api/v1beta1"
)

const (
	// workerpod's name derives from its owner processDump
	workernaming               = "kb-cdt-worker-%s"
	serviceAccountNaming       = "kb-cdt-worker-sa-%s"
	workerPodRolebindingNaming = "cdt-worker-rb-%s"
	clusterWorkerRoleName      = "cdt-worker-dumprole"
)

const (
	// SuccessSynced is used as part of the Event 'reason' when a ProcessDump is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a ProcessDump fails
	// to sync due to a Workerpod of the same name already existing.
	ErrResourceExists = "ErrResourceExists"
	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Worker already existing
	MessageResourceExists = "WorkerPod '%s' already exists and is not managed by ProcessDump"
	// MessageResourceSynced is the message used for an Event fired when a ProcessDump
	// is synced successfully
	MessageResourceSynced = "ProcessDump synced successfully"
)

// ProcessDumpReconciler reconciles a ProcessDump object
type ProcessDumpReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ProcessDumpReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var procdump diagnosticsofficecomv1beta1.ProcessDump
	var err error
	if err = r.Get(ctx, req.NamespacedName, &procdump); err != nil {
		klog.Error(err, "failed to get ProcessDump")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.Infof("%+v", procdump)

	// Ensure the serviceaccount used by workerpod exists
	var sa corev1.ServiceAccount
	serviceAccountKey := types.NamespacedName{
		Namespace: req.Namespace,
		Name:      fmt.Sprintf(serviceAccountNaming, req.Namespace),
	}
	err = r.Get(ctx, serviceAccountKey, &sa)
	if errors.IsNotFound(err) {
		// Need creating one
		klog.Infof("Create sa %q", serviceAccountKey)
		err = r.Create(ctx, newServiceAccount(req.Namespace))
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		klog.Warning("serviceaccount get/create failed")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure clusterrole exists
	var clusterRole rbacv1.ClusterRole
	clusterRoleKey := types.NamespacedName{
		Namespace: req.Namespace,
		Name:      clusterWorkerRoleName,
	}
	err = r.Get(ctx, clusterRoleKey, &clusterRole)
	if errors.IsNotFound(err) {
		klog.Infof("Create ClusterRole %q", clusterRoleKey)
		err = r.Create(ctx, newWorkPodClosterRole())
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		klog.Warning("clusterrole get/create failed")
		klog.Error(err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure clusterrolebinding exists
	var clusterRoleBinding rbacv1.ClusterRoleBinding
	clusterRoleBindingKey := types.NamespacedName{
		Namespace: req.Namespace,
		Name:      fmt.Sprintf(workerPodRolebindingNaming, req.Namespace),
	}
	err = r.Get(ctx, clusterRoleBindingKey, &clusterRoleBinding)
	if errors.IsNotFound(err) {
		klog.Infof("Create ClusterRoleBinding %q", clusterRoleBindingKey)
		err = r.Create(ctx, newClusterRoleBinding(req.Namespace))
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		klog.Warning("clusterrole get/create failed")
		klog.Error(err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Sync workerpod
	workerpodName := fmt.Sprintf(workernaming, req.Name)
	var workerpod corev1.Pod
	workerpodKey := types.NamespacedName{
		Namespace: req.Namespace,
		Name:      workerpodName,
	}
	err = r.Get(ctx, workerpodKey, &workerpod)
	if err == nil {
		// no err, indicate that user updated the ProcessDump, so re-create the workerpod.
		r.Delete(ctx, &workerpod)
		err = r.Create(ctx, newPod(&procdump))
		if err != nil && !errors.IsAlreadyExists(err) {
			klog.Warning("workerpod update failed")
			klog.Error(err.Error())
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	} else if errors.IsNotFound(err) {
		// If the pod doesn't exist, create it
		err = r.Create(ctx, newPod(&procdump))
		if err != nil && !errors.IsAlreadyExists(err) {
			klog.Warning("workerpod create failed")
			klog.Error(err.Error())
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}
	if err != nil && !errors.IsAlreadyExists(err) {
		klog.Warning("workerpod get/create failed")
		klog.Error(err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Finally, we update the status block of the ProcessDump resource to reflect the
	// current state of the world
	err = r.updateProcessDumpStatus(req, &workerpod)
	if err != nil {
		klog.Warning("processdump updatestatus failed")
		klog.Error(err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProcessDumpReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&diagnosticsofficecomv1beta1.ProcessDump{}).
		Complete(r)
}

// Updating workerpod name in processdump status
func (r *ProcessDumpReconciler) updateProcessDumpStatus(req ctrl.Request, workerPod *corev1.Pod) error {
	var err error
	var pd diagnosticsofficecomv1beta1.ProcessDump
	if err = r.Get(context.Background(), req.NamespacedName, &pd); err != nil {
		klog.Error(err.Error())
	}

	pd.Status.WorkerPodName = workerPod.Name
	if err = r.Status().Update(context.Background(), &pd); err != nil {
		klog.Error(err.Error())
	}

	return err
}

// return a workerpod based on processDump resource spec
func newPod(procdump *diagnosticsofficecomv1beta1.ProcessDump) *corev1.Pod {
	workerpodName := fmt.Sprintf(workernaming, procdump.Name)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerpodName,
			Namespace: procdump.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(procdump, diagnosticsofficecomv1beta1.GroupVersion.WithKind("ProcessDump")),
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "kubeclient",
					Image: "cosmictestacr.azurecr.io/cdt-worker:latest",
					Env: []corev1.EnvVar{
						{Name: "POD_NAME", Value: procdump.Spec.PodName},
						{Name: "CONTAINER_NAME", Value: procdump.Spec.ContainerName},
						{Name: "PROCESS_NAME", Value: procdump.Spec.ProcessName},
						{Name: "PROCESS_ID", Value: fmt.Sprintf("%d", procdump.Spec.ProcessID)},
						{Name: "PROCESS_DUMP_NAME", Value: procdump.Name},
						// kubeclient.NewEnvVar("NAMESPACE", "metadata.namespace"),
					},
				},
			},
			// NodeSelector:       kubeclient.NewNodeSelectorLinux(),
			NodeSelector:       map[string]string{"kubernetes.io/os": "linux"},
			ServiceAccountName: fmt.Sprintf(serviceAccountNaming, procdump.Namespace), //autumation creation,
		},
	}

	klog.Infof("Creating workerpod %s for processdump %s in namespace %s", workerpodName, procdump.Name, procdump.Namespace)
	return pod
}

func newWorkPodClosterRole() *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterWorkerRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"diagnostics.office.com"},
				Resources: []string{"processdumps", "processdumps/status"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		},
	}
	return clusterRole
}

func newClusterRoleBinding(namespace string) *rbacv1.ClusterRoleBinding {
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(workerPodRolebindingNaming, namespace),
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      fmt.Sprintf(serviceAccountNaming, namespace),
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterWorkerRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	return clusterRoleBinding
}

func newServiceAccount(namespace string) *corev1.ServiceAccount {
	serviceAccountName := fmt.Sprintf(serviceAccountNaming, namespace)
	serviceaccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}
	return serviceaccount
}
