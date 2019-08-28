package secrethub

import (
	"fmt"
	"strings"
	"sync"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"

	"github.com/secrethub/secrethub-go/internals/api"
	shaws "github.com/secrethub/secrethub-go/internals/aws"
	"github.com/secrethub/secrethub-go/internals/errio"
	"github.com/secrethub/secrethub-go/pkg/secrethub/credentials"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
)

// Errors
var (
	ErrInvalidAWSRegion      = errMain.Code("invalid_region").Error("invalid AWS region")
	ErrRoleAlreadyTaken      = errMain.Code("role_taken").Error("a service using that IAM role already exists")
	ErrInvalidPermissionPath = errMain.Code("invalid_permission_path").ErrorPref("invalid permission path: %s")
	ErrMissingRegion         = errMain.Code("missing_region").Error("could not find AWS region. Supply using the --region flag or in the AWS configuration. See https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html for the AWS configuration files")
)

// ServiceAWSInitCommand initializes a service for AWS.
type ServiceAWSInitCommand struct {
	description string
	repo        api.RepoPath
	kmsKeyID    string
	role        string
	region      string
	permission  string
	io          ui.IO
	newClient   newClientFunc
}

// NewServiceAWSInitCommand creates a new ServiceAWSInitCommand.
func NewServiceAWSInitCommand(io ui.IO, newClient newClientFunc) *ServiceAWSInitCommand {
	return &ServiceAWSInitCommand{
		io:        io,
		newClient: newClient,
	}
}

// Run initializes an AWS service.
func (cmd *ServiceAWSInitCommand) Run() error {
	client, err := cmd.newClient()
	if err != nil {
		return err
	}

	cfg := aws.NewConfig()
	if cmd.region != "" {
		_, ok := endpoints.AwsPartition().Regions()[cmd.region]
		if !ok {
			return ErrInvalidAWSRegion
		}
		cfg = cfg.WithRegion(cmd.region)
	}

	if cmd.role == "" {
		role, err := ui.Ask(cmd.io, "What role would you like to use?")
		if err != nil {
			return err
		}
		cmd.role = role
	}

	if cmd.kmsKeyID == "" {
		kmsKeyOptionsGetter := newKMSKeyOptionsGetter(cfg)
		kmsKey, err := ui.Choose(cmd.io, "What KMS Key would you like to use? Press [ENTER] for options.", kmsKeyOptionsGetter.get, true)
		if err != nil {
			return err
		}
		cmd.kmsKeyID = kmsKey
	}

	if cmd.description == "" {
		cmd.description = "AWS role " + roleNameFromRole(cmd.role)
	}

	service, err := client.Services().Create(cmd.repo.Value(), cmd.description, credentials.CreateAWS(cmd.kmsKeyID, cmd.role, cfg))
	if err == api.ErrCredentialAlreadyExists {
		return ErrRoleAlreadyTaken
	} else if err != nil {
		return err
	}

	err = givePermission(service, cmd.repo, cmd.permission, client)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.io.Stdout(), "The service %s is now reachable through AWS when the role %s is assumed.\n", service.ServiceID, cmd.role)

	return nil
}

// Register registers the command, arguments and flags on the provided Registerer.
func (cmd *ServiceAWSInitCommand) Register(r Registerer) {
	clause := r.Command("init", "Create a new AWS service account attached to a repository.")
	clause.Arg("repo", "The service account is attached to the repository in this path.").Required().SetValue(&cmd.repo)
	clause.Flag("kms-key-id", "ID of the KMS-key to be used for encrypting the service's account key.").StringVar(&cmd.kmsKeyID)
	clause.Flag("role", "ARN of the IAM role that should have access to this service account.").StringVar(&cmd.role)
	clause.Flag("region", "The AWS region that should be used").StringVar(&cmd.region)
	clause.Flag("description", "A description for the service so others will recognize it. Defaults to the name of the role that is attached to the service.").StringVar(&cmd.description)
	clause.Flag("descr", "").Hidden().StringVar(&cmd.description)
	clause.Flag("desc", "").Hidden().StringVar(&cmd.description)
	clause.Flag("permission", "Create an access rule giving the service account permission on a directory. Accepted permissions are `read`, `write` and `admin`. Use <permission> format to give permission on the root of the repo and <subdirectory>:<permission> to give permission on a subdirectory.").StringVar(&cmd.permission)

	BindAction(clause, cmd.Run)
}

func newKMSKeyOptionsGetter(cfg *aws.Config) kmsKeyOptionsGetter {
	return kmsKeyOptionsGetter{
		cfg:           cfg,
		timeFormatter: NewTimeFormatter(false),
	}
}

type kmsKeyOptionsGetter struct {
	cfg           *aws.Config
	timeFormatter TimeFormatter

	done       bool
	nextMarker string
}

func (g *kmsKeyOptionsGetter) get() ([]ui.Option, error) {
	if g.done {
		return []ui.Option{}, nil
	}

	listKeysInput := kms.ListKeysInput{}
	listKeysInput.SetLimit(10)
	if g.nextMarker != "" {
		listKeysInput.SetMarker(g.nextMarker)
	}

	sess, err := session.NewSession(g.cfg)
	if err != nil {
		return nil, err
	}
	kmsSvc := kms.New(sess)

	keys, err := kmsSvc.ListKeys(&listKeysInput)
	if err != nil {
		errAWS, ok := err.(awserr.Error)
		if ok {
			if errAWS.Code() == "NoCredentialProviders" {
				return nil, shaws.ErrNoAWSCredentials
			}
			if errAWS.Code() == "MissingRegion" {
				return nil, ErrMissingRegion
			}
			err = errio.Namespace("aws").Code(errAWS.Code()).Error(errAWS.Message())
		}
		return nil, fmt.Errorf("error fetching available KMS keys: %s", err)
	}

	if keys.NextMarker != nil {
		g.nextMarker = *keys.NextMarker
	} else {
		g.done = true
	}

	var waitgroup sync.WaitGroup
	options := make([]ui.Option, len(keys.Keys))

	for i, key := range keys.Keys {
		waitgroup.Add(1)
		go func(i int, key *kms.KeyListEntry) {
			option := ui.Option{
				Value:   aws.StringValue(key.KeyArn),
				Display: aws.StringValue(key.KeyId),
			}

			resp, err := kmsSvc.DescribeKey(&kms.DescribeKeyInput{KeyId: key.KeyId})
			if err == nil {
				option.Display += "\t" + aws.StringValue(resp.KeyMetadata.Description) + "\t" + g.timeFormatter.Format(*resp.KeyMetadata.CreationDate)
			}

			options[i] = option
			waitgroup.Done()
		}(i, key)
	}
	waitgroup.Wait()

	return options, nil
}

// roleNameFromRole returns the name of the role indicated by the input. Accepted input is:
// - A role name (e.g. my-role)
// - A role name, prefixed by "role/" (e.g. role/my-role)
// - A role ARN (e.g. arn:aws:iam::123456789012:role/my-role)
//
// When the input is not one of these accepted inputs, no guarantees about the expected return
// are made.
func roleNameFromRole(role string) string {
	if strings.Contains(role, ":") {
		parts := strings.SplitN(role, "role/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
		return ""
	}
	return strings.TrimPrefix(role, "role/")
}
