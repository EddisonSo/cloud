package k8s

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
}

func NewClient() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &Client{clientset: clientset, config: config}, nil
}

// CreateNamespace creates a namespace for a container
func (c *Client) CreateNamespace(ctx context.Context, name string, userID int64, containerID string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"edd-compute":  "true",
				"user-id":      fmt.Sprintf("%d", userID),
				"container-id": containerID,
			},
		},
	}

	_, err := c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace: %w", err)
	}
	return nil
}

// DeleteNamespace deletes a container namespace and all resources in it
func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	err := c.clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete namespace: %w", err)
	}
	return nil
}

// CreateSSHSecret creates a secret with SSH authorized_keys
func (c *Client) CreateSSHSecret(ctx context.Context, namespace string, authorizedKeys string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ssh-keys",
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"authorized_keys": authorizedKeys,
		},
	}

	_, err := c.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create ssh secret: %w", err)
	}
	return nil
}

// CreatePVC creates a persistent volume claim for container storage
func (c *Client) CreatePVC(ctx context.Context, namespace string, storageGB int) error {
	storageClassName := "local-path"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "storage",
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: parseQuantity(fmt.Sprintf("%dGi", storageGB)),
				},
			},
		},
	}

	_, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create pvc: %w", err)
	}
	return nil
}

// CreateNetworkPolicy creates network isolation policy (blocks all external ingress by default)
func (c *Client) CreateNetworkPolicy(ctx context.Context, namespace string) error {
	return c.UpdateNetworkPolicy(ctx, namespace, nil) // Start with no ports open
}

// UpdateNetworkPolicy updates the network policy to allow only specified ports from external sources
func (c *Client) UpdateNetworkPolicy(ctx context.Context, namespace string, allowedPorts []int) error {
	udpProtocol := corev1.ProtocolUDP
	tcpProtocol := corev1.ProtocolTCP
	dnsPort := int32(53)

	// Build ingress rules
	var ingressRules []networkingv1.NetworkPolicyIngressRule

	// Always allow ingress from within the cluster (for cloud terminal via internal pod IP)
	ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{
			{
				IPBlock: &networkingv1.IPBlock{
					CIDR: "10.0.0.0/8", // Internal cluster network
				},
			},
		},
	})

	// Always allow gateway to connect (gateway routes SSH/HTTP/HTTPS through itself)
	// The gateway enforces protocol access at the routing level, NetworkPolicy just allows the connection
	ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{
			{
				IPBlock: &networkingv1.IPBlock{
					CIDR: "192.168.3.200/32", // Gateway LoadBalancer IP
				},
			},
		},
	})

	// Add rules for each allowed external port (for direct access bypassing gateway)
	for _, port := range allowedPorts {
		p := int32(port)
		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "0.0.0.0/0", // External traffic
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &tcpProtocol,
					Port:     &intOrString{IntVal: p},
				},
			},
		})
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "isolation",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingressRules,
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// Allow DNS
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &udpProtocol,
							Port:     &intOrString{IntVal: dnsPort},
						},
					},
				},
				{
					// Allow internal cluster traffic (for responses to compute service, etc.)
					To: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "10.0.0.0/8",
							},
						},
					},
				},
				{
					// Allow internet (external traffic)
					To: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR:   "0.0.0.0/0",
								Except: []string{"10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12"},
							},
						},
					},
				},
			},
		},
	}

	// Try to update, if not exists then create
	_, err := c.clientset.NetworkingV1().NetworkPolicies(namespace).Update(ctx, policy, metav1.UpdateOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = c.clientset.NetworkingV1().NetworkPolicies(namespace).Create(ctx, policy, metav1.CreateOptions{})
		}
		if err != nil {
			return fmt.Errorf("update network policy: %w", err)
		}
	}
	return nil
}

// CreatePod creates the container pod with user-specified mount paths
func (c *Client) CreatePod(ctx context.Context, namespace string, image string, memoryMB int, arch string, cpuCores string, mountPaths []string) error {
	defaultMode := int32(0600)

	// Build subpath names for each mount path
	var subPaths []string
	for i, path := range mountPaths {
		subPath := strings.TrimPrefix(path, "/")
		subPath = strings.ReplaceAll(subPath, "/", "-")
		if subPath == "" {
			subPath = fmt.Sprintf("mount-%d", i)
		}
		subPaths = append(subPaths, subPath)
	}

	// Build volume mounts for each user-specified path using subpaths
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "ssh-keys",
			MountPath: "/etc/ssh/keys",
			ReadOnly:  true,
		},
	}
	for i, path := range mountPaths {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "storage",
			MountPath: path,
			SubPath:   subPaths[i],
		})
	}

	// Build mkdir command for init container to create subpath directories
	mkdirCmd := "cd /mnt/storage"
	for _, sp := range subPaths {
		mkdirCmd += fmt.Sprintf(" && mkdir -p %s", sp)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "container",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "compute-container",
			},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"compute-schedulable": "true",
				"kubernetes.io/arch":  arch,
			},
			InitContainers: []corev1.Container{
				{
					Name:    "init-storage",
					Image:   "busybox:latest",
					Command: []string{"/bin/sh", "-c", mkdirCmd},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "storage",
							MountPath: "/mnt/storage",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: image,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: parseQuantity(fmt.Sprintf("%dMi", memoryMB)),
							corev1.ResourceCPU:    parseQuantity(cpuCores),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: parseQuantity(fmt.Sprintf("%dMi", memoryMB)),
							corev1.ResourceCPU:    parseQuantity(cpuCores),
						},
					},
					Ports: []corev1.ContainerPort{
						{ContainerPort: 22, Name: "ssh"},
					},
					VolumeMounts: volumeMounts,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "storage",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "storage",
						},
					},
				},
				{
					Name: "ssh-keys",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "ssh-keys",
							DefaultMode: &defaultMode,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}

	_, err := c.clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create pod: %w", err)
	}
	return nil
}

// CreateLoadBalancer creates a LoadBalancer service for the container
func (c *Client) CreateLoadBalancer(ctx context.Context, namespace string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lb",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": "compute-container",
			},
			Ports: []corev1.ServicePort{
				{Name: "ssh", Port: 22, TargetPort: intOrString{IntVal: 22}},
				{Name: "http", Port: 80, TargetPort: intOrString{IntVal: 80}},
				{Name: "https", Port: 443, TargetPort: intOrString{IntVal: 443}},
				{Name: "dev-3000", Port: 3000, TargetPort: intOrString{IntVal: 3000}},
				{Name: "dev-8080", Port: 8080, TargetPort: intOrString{IntVal: 8080}},
			},
		},
	}

	_, err := c.clientset.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create load balancer: %w", err)
	}
	return nil
}

// UpdateLoadBalancerPorts updates the LoadBalancer service to include the specified ports
func (c *Client) UpdateLoadBalancerPorts(ctx context.Context, namespace string, ports []int) error {
	svc, err := c.clientset.CoreV1().Services(namespace).Get(ctx, "lb", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get service: %w", err)
	}

	// Build new ports list: keep base ports (22, 80, 443) and add user-defined ports
	basePorts := map[int]bool{22: true, 80: true, 443: true}
	newPorts := []corev1.ServicePort{
		{Name: "ssh", Port: 22, TargetPort: intOrString{IntVal: 22}, Protocol: corev1.ProtocolTCP},
		{Name: "http", Port: 80, TargetPort: intOrString{IntVal: 80}, Protocol: corev1.ProtocolTCP},
		{Name: "https", Port: 443, TargetPort: intOrString{IntVal: 443}, Protocol: corev1.ProtocolTCP},
	}

	// Add user-defined ports
	for _, port := range ports {
		if basePorts[port] {
			continue // Skip base ports
		}
		newPorts = append(newPorts, corev1.ServicePort{
			Name:       fmt.Sprintf("port-%d", port),
			Port:       int32(port),
			TargetPort: intOrString{IntVal: int32(port)},
			Protocol:   corev1.ProtocolTCP,
		})
	}

	svc.Spec.Ports = newPorts
	_, err = c.clientset.CoreV1().Services(namespace).Update(ctx, svc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}
	return nil
}

// GetServiceExternalIP gets the external IP of a LoadBalancer service
func (c *Client) GetServiceExternalIP(ctx context.Context, namespace string) (string, error) {
	svc, err := c.clientset.CoreV1().Services(namespace).Get(ctx, "lb", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get service: %w", err)
	}

	if len(svc.Status.LoadBalancer.Ingress) > 0 {
		if svc.Status.LoadBalancer.Ingress[0].IP != "" {
			return svc.Status.LoadBalancer.Ingress[0].IP, nil
		}
	}
	return "", nil // No IP assigned yet
}

// GetPodStatus gets the status of a pod, checking container readiness
func (c *Client) GetPodStatus(ctx context.Context, namespace string) (string, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, "container", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "not_found", nil
		}
		return "", fmt.Errorf("get pod: %w", err)
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		return "pending", nil
	case corev1.PodRunning:
		// Check if all containers are ready
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.ContainersReady && cond.Status == corev1.ConditionTrue {
				return "running", nil
			}
		}
		// Pod is running but containers not ready yet
		return "initializing", nil
	case corev1.PodSucceeded:
		return "stopped", nil
	case corev1.PodFailed:
		return "failed", nil
	default:
		return "unknown", nil
	}
}

// GetPodIP returns the internal cluster IP of the container pod
func (c *Client) GetPodIP(ctx context.Context, namespace string) (string, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, "container", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod: %w", err)
	}
	return pod.Status.PodIP, nil
}

// DeletePod deletes the container pod
func (c *Client) DeletePod(ctx context.Context, namespace string) error {
	err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, "container", metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete pod: %w", err)
	}
	return nil
}

// GetGatewayPublicKey retrieves the gateway SSH public key from the K8s Secret
func (c *Client) GetGatewayPublicKey(ctx context.Context) (string, error) {
	secret, err := c.clientset.CoreV1().Secrets("default").Get(ctx, "gateway-ssh-key", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil // Secret doesn't exist yet
		}
		return "", fmt.Errorf("get gateway secret: %w", err)
	}

	pubKey, ok := secret.Data["public_key"]
	if !ok {
		return "", fmt.Errorf("gateway secret missing public_key field")
	}

	return string(pubKey), nil
}

// InjectTempKey writes a temporary SSH public key to the container for cloud terminal access.
// The temp-key-daemon inside the container will pick up this key and add it to authorized_keys.
func (c *Client) InjectTempKey(ctx context.Context, namespace, pubKey, keyID string) error {
	// Ensure temp-keys directory exists and write the key file
	cmd := []string{
		"/bin/sh", "-c",
		fmt.Sprintf("mkdir -p /tmp/temp-keys && echo '%s' > /tmp/temp-keys/%s",
			strings.ReplaceAll(pubKey, "'", "'\"'\"'"), keyID),
	}

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name("container").
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "main",
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("exec failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// ResourceUsage holds memory and disk usage for a container
type ResourceUsage struct {
	MemoryUsedMB  int64
	StorageUsedGB float64
}

// GetResourceUsage gets memory and disk usage for a container pod
func (c *Client) GetResourceUsage(ctx context.Context, namespace string) (*ResourceUsage, error) {
	usage := &ResourceUsage{}

	// Get memory usage from pod metrics (requires metrics-server)
	// Use raw REST client to query metrics API
	result := c.clientset.CoreV1().RESTClient().Get().
		AbsPath("/apis/metrics.k8s.io/v1beta1").
		Namespace(namespace).
		Resource("pods").
		Name("container").
		Do(ctx)

	if result.Error() == nil {
		raw, err := result.Raw()
		if err == nil {
			// Parse memory from response - format: {"containers":[{"usage":{"memory":"123456Ki"}}]}
			memStr := extractJSONField(string(raw), "memory")
			if memStr != "" {
				usage.MemoryUsedMB = parseMemoryToMB(memStr)
			}
		}
	}

	// Get disk usage by exec'ing df command
	cmd := []string{"/bin/sh", "-c", "df -B1 /root 2>/dev/null | tail -1 | awk '{print $3}'"}
	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name("container").
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "main",
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err == nil {
		var stdout, stderr bytes.Buffer
		if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
		}); err == nil {
			// Parse bytes used
			var bytesUsed int64
			fmt.Sscanf(strings.TrimSpace(stdout.String()), "%d", &bytesUsed)
			usage.StorageUsedGB = float64(bytesUsed) / (1024 * 1024 * 1024)
		}
	}

	return usage, nil
}

// NamespaceInfo holds information about a K8s namespace
type NamespaceInfo struct {
	Name        string
	Labels      map[string]string
	CreatedAt   string
	ContainerID string
	UserID      string
}

// ListAllNamespaces returns all namespaces in the cluster
func (c *Client) ListAllNamespaces(ctx context.Context) ([]NamespaceInfo, error) {
	nsList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	result := make([]NamespaceInfo, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		info := NamespaceInfo{
			Name:      ns.Name,
			Labels:    ns.Labels,
			CreatedAt: ns.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		}
		if ns.Labels != nil {
			info.ContainerID = ns.Labels["container-id"]
			info.UserID = ns.Labels["user-id"]
		}
		result = append(result, info)
	}
	return result, nil
}

// extractJSONField is a simple helper to extract a field value from JSON
func extractJSONField(json, field string) string {
	key := fmt.Sprintf(`"%s":"`, field)
	idx := strings.Index(json, key)
	if idx == -1 {
		return ""
	}
	start := idx + len(key)
	end := strings.Index(json[start:], `"`)
	if end == -1 {
		return ""
	}
	return json[start : start+end]
}

// parseMemoryToMB converts K8s memory string (e.g., "123456Ki") to MB
func parseMemoryToMB(mem string) int64 {
	mem = strings.TrimSpace(mem)
	var value int64
	var unit string
	fmt.Sscanf(mem, "%d%s", &value, &unit)

	switch strings.ToLower(unit) {
	case "ki":
		return value / 1024
	case "mi":
		return value
	case "gi":
		return value * 1024
	case "k":
		return value / 1000
	case "m":
		return value
	case "g":
		return value * 1000
	default:
		// Assume bytes
		return value / (1024 * 1024)
	}
}
