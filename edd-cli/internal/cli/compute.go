package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func init() { register(command{name: "compute", run: cmdCompute}) }

func containerTable(w io.Writer, cs []eddsdk.Container) {
	rows := make([][]string, len(cs))
	for i, c := range cs {
		rows[i] = []string{c.ID, c.Name, c.Status, c.InstanceType, c.PullPolicy}
	}
	printTable(w, []string{"ID", "NAME", "STATUS", "TYPE", "PULL"}, rows)
}

func cmdCompute(c *eddsdk.Client, _ string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec compute <containers|keys> <action> [args]")
	}
	switch args[0] {
	case "containers":
		return cmdComputeContainers(c, args[1:])
	case "keys":
		return cmdComputeKeys(c, args[1:])
	default:
		return fmt.Errorf("unknown compute resource: %s", args[0])
	}
}

func cmdComputeContainers(c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec compute containers <ls|get|create|start|stop|rm|logs|pull-policy|ssh|ingress|mounts> [args]")
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		cs, err := c.ListContainers(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(cs)
		}
		containerTable(os.Stdout, cs)
		return nil
	case "get":
		if len(rest) < 1 {
			return fmt.Errorf("usage: ec compute containers get <id>")
		}
		ct, err := c.GetContainer(ctx, rest[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(ct)
		}
		containerTable(os.Stdout, []eddsdk.Container{*ct})
		return nil
	case "start":
		return needID(rest, func(id string) error { return c.StartContainer(ctx, id) })
	case "stop":
		return needID(rest, func(id string) error { return c.StopContainer(ctx, id) })
	case "rm":
		return needID(rest, func(id string) error { return c.DeleteContainer(ctx, id) })
	case "pull-policy":
		if len(rest) != 2 {
			return fmt.Errorf("usage: ec compute containers pull-policy <id> <Always|IfNotPresent>")
		}
		return c.SetPullPolicy(ctx, rest[0], rest[1])
	case "ssh":
		if len(rest) != 2 || (rest[1] != "on" && rest[1] != "off") {
			return fmt.Errorf("usage: ec compute containers ssh <id> <on|off>")
		}
		return c.SetSSH(ctx, rest[0], rest[1] == "on")
	case "logs":
		if len(rest) < 1 {
			return fmt.Errorf("usage: ec compute containers logs <id>")
		}
		out, err := c.ContainerLogs(ctx, rest[0])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	case "ingress":
		return cmdComputeIngress(ctx, c, rest)
	case "mounts":
		return cmdComputeMounts(ctx, c, rest)
	case "create":
		return cmdComputeCreate(ctx, c, rest)
	default:
		return fmt.Errorf("unknown containers action: %s", sub)
	}
}

func needID(args []string, fn func(string) error) error {
	if len(args) < 1 {
		return fmt.Errorf("a container id is required")
	}
	return fn(args[0])
}

func cmdComputeIngress(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec compute containers ingress <ls|add|rm> <id> [args]")
	}
	switch args[0] {
	case "ls":
		if len(args) < 2 {
			return fmt.Errorf("usage: ec compute containers ingress ls <id>")
		}
		rules, err := c.ListIngress(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(rules)
		}
		rows := make([][]string, len(rules))
		for i, r := range rules {
			rows[i] = []string{strconv.Itoa(r.Port), strconv.Itoa(r.TargetPort)}
		}
		printTable(os.Stdout, []string{"PORT", "TARGET"}, rows)
		return nil
	case "add":
		if len(args) != 4 {
			return fmt.Errorf("usage: ec compute containers ingress add <id> <port> <target>")
		}
		port, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		target, err := strconv.Atoi(args[3])
		if err != nil {
			return fmt.Errorf("invalid target: %w", err)
		}
		return c.AddIngress(ctx, args[1], port, target)
	case "rm":
		if len(args) != 3 {
			return fmt.Errorf("usage: ec compute containers ingress rm <id> <port>")
		}
		port, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		return c.RemoveIngress(ctx, args[1], port)
	default:
		return fmt.Errorf("unknown ingress subcommand: %s", args[0])
	}
}

func cmdComputeMounts(ctx context.Context, c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec compute containers mounts <ls|set> <id> [paths...]")
	}
	switch args[0] {
	case "ls":
		if len(args) < 2 {
			return fmt.Errorf("usage: ec compute containers mounts ls <id>")
		}
		paths, err := c.GetMounts(ctx, args[1])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(paths)
		}
		for _, p := range paths {
			fmt.Println(p)
		}
		return nil
	case "set":
		if len(args) < 2 {
			return fmt.Errorf("usage: ec compute containers mounts set <id> [path...]")
		}
		return c.SetMounts(ctx, args[1], args[2:])
	default:
		return fmt.Errorf("unknown mounts subcommand: %s", args[0])
	}
}

func cmdComputeCreate(ctx context.Context, c *eddsdk.Client, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	name := fs.String("name", "", "container name")
	image := fs.String("image", "", "image (registry.cloud.eddisonso.com/...)")
	itype := fs.String("type", "nano", "instance type")
	mem := fs.Int("memory", 256, "memory MB")
	storage := fs.Int("storage", 5, "storage GB")
	pull := fs.String("pull-policy", "IfNotPresent", "Always|IfNotPresent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	ct, err := c.CreateContainer(ctx, eddsdk.CreateContainerRequest{
		Name: *name, Image: *image, InstanceType: *itype,
		MemoryMB: *mem, StorageGB: *storage, PullPolicy: *pull,
	})
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(ct)
	}
	fmt.Printf("created %s (%s)\n", ct.Name, ct.ID)
	return nil
}

// --- keys resource ---

func sshKeyTable(w io.Writer, keys []eddsdk.SSHKey) {
	rows := make([][]string, len(keys))
	for i, k := range keys {
		rows[i] = []string{strconv.FormatInt(k.ID, 10), k.Name, k.CreatedAt}
	}
	printTable(w, []string{"ID", "NAME", "CREATED_AT"}, rows)
}

func cmdComputeKeys(c *eddsdk.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ec compute keys <ls|add|rm> [args]")
	}
	ctx := context.Background()
	switch args[0] {
	case "ls":
		keys, err := c.ListSSHKeys(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(keys)
		}
		sshKeyTable(os.Stdout, keys)
		return nil
	case "add":
		return cmdComputeKeysAdd(ctx, c, args[1:])
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: ec compute keys rm <id>")
		}
		return c.DeleteSSHKey(ctx, args[1])
	default:
		return fmt.Errorf("unknown keys action: %s", args[0])
	}
}

func cmdComputeKeysAdd(ctx context.Context, c *eddsdk.Client, args []string) error {
	fs := flag.NewFlagSet("keys add", flag.ContinueOnError)
	name := fs.String("name", "", "key name (required)")
	key := fs.String("key", "", "SSH public key string (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *key == "" {
		return fmt.Errorf("--key is required")
	}
	k, err := c.AddSSHKey(ctx, *name, *key)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(k)
	}
	fmt.Printf("added SSH key %s (id=%d)\n", k.Name, k.ID)
	return nil
}
