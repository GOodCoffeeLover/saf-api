package provisioning

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

// Cmd defines a shell command.
type Cmd struct {
	Cmd   string
	Args  []string
	Stdin string
}

// UnmarshalJSON a runcmd command
// It can be either a list or a string.
// If the item is a list, the head of the list is the command and the tail are the args.
// If the item is a string, the whole command will be wrapped in `/bin/sh -c`.
func (c *Cmd) UnmarshalJSON(data []byte) error {
	// First, try to decode the input as a list
	var s1 []string
	if err := json.Unmarshal(data, &s1); err != nil {
		if _, ok := err.(*json.UnmarshalTypeError); !ok {
			return errors.WithStack(err)
		}
	} else {
		c.Cmd = s1[0]
		c.Args = s1[1:]
		return nil
	}

	// If it's not a list, it must be a string
	var s2 string
	if err := json.Unmarshal(data, &s2); err != nil {
		return errors.WithStack(err)
	}

	c.Cmd = "/bin/sh"
	c.Args = []string{"-c", s2}

	return nil
}

const (
	// Supported cloud config modules.
	writefiles = "write_files"
	runcmd     = "runcmd"
)

type actionFactory struct{}

func (a *actionFactory) action(name string) action {
	switch name {
	case writefiles:
		return newWriteFilesAction()
	case runcmd:
		return newRunCmdAction()
	default:
		// TODO Add a logger during the refactor and log this unknown module
		return newUnknown(name)
	}
}

type action interface {
	Unmarshal(userData []byte) error
	Commands() ([]Cmd, error)
}

type unknown struct {
	module string
	lines  []string
}

func newUnknown(module string) action {
	return &unknown{module: module}
}

// Unmarshal will unmarshal unknown actions and slurp the value.
func (u *unknown) Unmarshal(data []byte) error {
	// try unmarshalling to a slice of strings
	var s1 []string
	if err := json.Unmarshal(data, &s1); err != nil {
		if _, ok := err.(*json.UnmarshalTypeError); !ok {
			return errors.WithStack(err)
		}
	} else {
		u.lines = s1
		return nil
	}

	// If it's not a slice of strings it should be one string value
	var s2 string
	if err := json.Unmarshal(data, &s2); err != nil {
		return errors.WithStack(err)
	}

	u.lines = []string{s2}
	return nil
}

func (u *unknown) Commands() ([]Cmd, error) {
	return []Cmd{}, nil
}

// RawCloudInitToProvisioningCommands converts a cloudconfig to a list of commands to run in sequence on the node.
func RawCloudInitToProvisioningCommands(config []byte) ([]Cmd, error) {
	// validate cloudConfigScript is a valid yaml, as required by the cloud config specification
	if err := yaml.Unmarshal(config, &map[string]interface{}{}); err != nil {
		return nil, errors.Wrapf(err, "cloud-config is not valid yaml")
	}

	// parse the cloud config yaml into a slice of cloud config actions.
	actions, err := getActions(config)
	if err != nil {
		return nil, err
	}

	commands := []Cmd{}
	for _, action := range actions {
		cmds, err := action.Commands()
		if err != nil {
			return commands, err
		}
		commands = append(commands, cmds...)
	}

	return commands, nil
}

// getActions parses the cloud config yaml into a slice of actions to run.
// Parsing manually is required because the order of the cloud config's actions must be maintained.
func getActions(userData []byte) ([]action, error) {
	actionRegEx := regexp.MustCompile(`^[a-zA-Z_]*:`)
	lines := make([]string, 0)
	actions := make([]action, 0)
	actionFactory := &actionFactory{}

	var act action

	// scans the file searching for keys/top level actions.
	scanner := bufio.NewScanner(bytes.NewReader(userData))
	for scanner.Scan() {
		line := scanner.Text()
		// if the line is key/top level action
		if actionRegEx.MatchString(line) {
			// converts the file fragment scanned up to now into the current action, if any
			if act != nil {
				actionBlock := strings.Join(lines, "\n")
				if err := act.Unmarshal([]byte(actionBlock)); err != nil {
					return nil, errors.WithStack(err)
				}
				actions = append(actions, act)
				lines = lines[:0]
			}

			// creates the new action
			actionName := strings.TrimSuffix(line, ":")
			act = actionFactory.action(actionName)
		}

		lines = append(lines, line)
	}

	// converts the last file fragment scanned into the current action, if any
	if act != nil {
		actionBlock := strings.Join(lines, "\n")
		if err := act.Unmarshal([]byte(actionBlock)); err != nil {
			return nil, errors.WithStack(err)
		}
		actions = append(actions, act)
	}

	return actions, scanner.Err()
}

// runCmd defines parameters of a shell command that is equivalent to an action found in the cloud init rundcmd module.
type runCmd struct {
	Cmds []Cmd `json:"runcmd,"`
}

func newRunCmdAction() action {
	return &runCmd{}
}

// Unmarshal the runCmd.
func (a *runCmd) Unmarshal(userData []byte) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(err, "error parsing run_cmd action: %s", userData)
	}
	return nil
}

// Commands returns a list of commands to run on the node.
func (a *runCmd) Commands() ([]Cmd, error) {
	return a.Cmds, nil
}

// writeFilesAction defines a list of files that should be written to a node.
type writeFilesAction struct {
	Files []files `json:"write_files,"`
}

type files struct {
	Path        string `json:"path,"`
	Encoding    string `json:"encoding,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Permissions string `json:"permissions,omitempty"`
	Content     string `json:"content,"`
	Append      bool   `json:"append,"`
}

func newWriteFilesAction() action {
	return &writeFilesAction{}
}

func (a *writeFilesAction) Unmarshal(userData []byte) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(err, "error parsing write_files action: %s", userData)
	}
	return nil
}

// Commands return a list of commands to run on the node.
// Each command defines the parameters of a shell command necessary to generate a file replicating the cloud-init write_files module.
func (a *writeFilesAction) Commands() ([]Cmd, error) {
	commands := make([]Cmd, 0)
	for _, f := range a.Files {
		// Fix attributes and apply defaults
		path := fixPath(f.Path) // NB. the real cloud init module for writes files converts path into absolute paths; this is not possible here...
		encodings := fixEncoding(f.Encoding)
		owner := fixOwner(f.Owner)
		permissions := fixPermissions(f.Permissions)
		content, err := fixContent(f.Content, encodings)
		if err != nil {
			return commands, errors.Wrapf(err, "error decoding content for %s", path)
		}

		// Make the directory so cat + redirection will work
		directory := filepath.Dir(path)
		commands = append(commands, Cmd{Cmd: "mkdir", Args: []string{"-p", directory}})

		redirects := ">"
		if f.Append {
			redirects = ">>"
		}

		// generate a command that will create a file with the expected contents.
		commands = append(commands, Cmd{Cmd: "/bin/sh", Args: []string{"-c", fmt.Sprintf("cat %s %s /dev/stdin", redirects, path)}, Stdin: content})

		// if permissions are different than default ownership, add a command to modify the permissions.
		if permissions != "0644" {
			commands = append(commands, Cmd{Cmd: "chmod", Args: []string{permissions, path}})
		}

		// if ownership is different than default ownership, add a command to modify file ownerhsip.
		if owner != "root:root" {
			commands = append(commands, Cmd{Cmd: "chown", Args: []string{owner, path}})
		}
	}
	return commands, nil
}

func fixPath(p string) string {
	return strings.TrimSpace(p)
}

func fixOwner(o string) string {
	o = strings.TrimSpace(o)
	if o != "" {
		return o
	}
	return "root:root"
}

func fixPermissions(p string) string {
	p = strings.TrimSpace(p)
	if p != "" {
		return p
	}
	return "0644"
}

func fixEncoding(e string) []string {
	e = strings.ToLower(e)
	e = strings.TrimSpace(e)

	switch e {
	case "gz", "gzip":
		return []string{"application/x-gzip"}
	case "gz+base64", "gzip+base64", "gz+b64", "gzip+b64":
		return []string{"application/base64", "application/x-gzip"}
	case "base64", "b64":
		return []string{"application/base64"}
	}

	return []string{"text/plain"}
}

func fixContent(content string, encodings []string) (string, error) {
	for _, e := range encodings {
		switch e {
		case "application/base64":
			rByte, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				return content, errors.WithStack(err)
			}
			return string(rByte), nil
		case "application/x-gzip":
			rByte, err := gUnzipData([]byte(content))
			if err != nil {
				return content, err
			}
			return string(rByte), nil
		case "text/plain":
			return content, nil
		default:
			return content, errors.Errorf("Unknown bootstrap data encoding: %q", content)
		}
	}
	return content, nil
}

func gUnzipData(data []byte) ([]byte, error) {
	var r io.Reader
	var err error
	b := bytes.NewBuffer(data)
	r, err = gzip.NewReader(b)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return resB.Bytes(), nil
}
