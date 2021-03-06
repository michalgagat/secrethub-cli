package secrethub

import (
	"errors"
	"testing"
	"time"

	"github.com/secrethub/secrethub-cli/internals/cli/ui"
	"github.com/secrethub/secrethub-cli/internals/secrethub/fakes"
	"github.com/secrethub/secrethub-go/internals/api"
	"github.com/secrethub/secrethub-go/internals/assert"
	"github.com/secrethub/secrethub-go/pkg/secrethub"
	"github.com/secrethub/secrethub-go/pkg/secrethub/fakeclient"
)

func TestAuditRepoCommand_run(t *testing.T) {
	testError := errors.New("test error")

	cases := map[string]struct {
		cmd AuditCommand
		err error
		out string
	}{
		"0 events": {
			cmd: AuditCommand{
				path: "namespace/repo",
				newClient: func() (secrethub.ClientInterface, error) {
					return &fakeclient.Client{
						DirService: &fakeclient.DirService{
							GetTreeFunc: func(path string, depth int, ancestors bool) (*api.Tree, error) {
								return nil, nil
							},
						},
						RepoService: &fakeclient.RepoService{
							AuditEventIterator: &fakeclient.AuditEventIterator{
								Events: []api.Audit{},
							},
						},
					}, nil
				},
				perPage: 20,
			},
			out: "AUTHOR    EVENT    EVENT SUBJECT    IP ADDRESS    DATE\n",
		},
		"create repo event": {
			cmd: AuditCommand{
				path: "namespace/repo",
				newClient: func() (secrethub.ClientInterface, error) {
					return fakeclient.Client{
						DirService: &fakeclient.DirService{
							GetTreeFunc: func(path string, depth int, ancestors bool) (*api.Tree, error) {
								return nil, nil
							},
						},
						RepoService: &fakeclient.RepoService{
							AuditEventIterator: &fakeclient.AuditEventIterator{
								Events: []api.Audit{
									{
										Action: "create",
										Actor: api.AuditActor{
											Type: "user",
											User: &api.User{
												Username: "developer",
											},
										},
										LoggedAt: time.Date(2018, 1, 1, 1, 1, 1, 1, time.UTC),
										Subject: api.AuditSubject{
											Type: "repo",
											Repo: &api.Repo{
												Name: "repo",
											},
										},
										IPAddress: "127.0.0.1",
									},
								},
							},
						},
					}, nil
				},
				perPage: 20,
				timeFormatter: &fakes.TimeFormatter{
					Response: "2018-01-01T01:01:01+01:00",
				},
			},
			out: "AUTHOR       EVENT          EVENT SUBJECT    IP ADDRESS    DATE\n" +
				"developer    create.repo    repo             127.0.0.1     2018-01-01T01:01:01+01:00\n",
		},
		"client creation error": {
			cmd: AuditCommand{
				path: "namespace/repo",
				newClient: func() (secrethub.ClientInterface, error) {
					return nil, ErrCannotFindHomeDir()
				},
				perPage: 20,
			},
			err: ErrCannotFindHomeDir(),
		},
		"list audit events error": {
			cmd: AuditCommand{
				path: "namespace/repo",
				newClient: func() (secrethub.ClientInterface, error) {
					return fakeclient.Client{
						RepoService: &fakeclient.RepoService{
							AuditEventIterator: &fakeclient.AuditEventIterator{
								Err: testError,
							},
						},
						DirService: &fakeclient.DirService{
							GetTreeFunc: func(path string, depth int, ancestors bool) (*api.Tree, error) {
								return nil, nil
							},
						},
					}, nil
				},
				perPage: 20,
			},
			err: testError,
		},
		"get dir error": {
			cmd: AuditCommand{
				path: "namespace/repo",
				newClient: func() (secrethub.ClientInterface, error) {
					return fakeclient.Client{
						DirService: &fakeclient.DirService{
							GetTreeFunc: func(path string, depth int, ancestors bool) (*api.Tree, error) {
								return nil, testError
							},
						},
						RepoService: &fakeclient.RepoService{
							ListEventsFunc: func(path string, subjectTypes api.AuditSubjectTypeList) ([]*api.Audit, error) {
								return nil, nil
							},
						},
					}, nil
				},
				perPage: 20,
			},
			err: testError,
		},
		"invalid audit actor": {
			cmd: AuditCommand{
				path: "namespace/repo",
				newClient: func() (secrethub.ClientInterface, error) {
					return fakeclient.Client{
						DirService: &fakeclient.DirService{
							GetTreeFunc: func(path string, depth int, ancestors bool) (*api.Tree, error) {
								return nil, nil
							},
						},
						RepoService: &fakeclient.RepoService{
							AuditEventIterator: &fakeclient.AuditEventIterator{
								Events: []api.Audit{
									{
										Subject: api.AuditSubject{
											Type: api.AuditSubjectService,
											Service: &api.Service{
												ServiceID: "<service id>",
											},
										},
									},
								},
							},
						},
					}, nil
				},
				perPage: 20,
			},
			err: ErrInvalidAuditActor,
			out: "",
		},
		"invalid audit subject": {
			cmd: AuditCommand{
				path: "namespace/repo",
				newClient: func() (secrethub.ClientInterface, error) {
					return fakeclient.Client{
						DirService: &fakeclient.DirService{
							GetTreeFunc: func(path string, depth int, ancestors bool) (*api.Tree, error) {
								return nil, nil
							},
						},
						RepoService: &fakeclient.RepoService{
							AuditEventIterator: &fakeclient.AuditEventIterator{
								Events: []api.Audit{
									{
										Actor: api.AuditActor{
											Type: "user",
											User: &api.User{
												Username: "developer",
											},
										},
									},
								},
							},
						},
					}, nil
				},
				perPage: 20,
			},
			err: ErrInvalidAuditSubject,
			out: "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Setup
			io := ui.NewFakeIO()
			tc.cmd.io = io

			// Act
			err := tc.cmd.run()

			// Assert
			assert.Equal(t, err, tc.err)
			assert.Equal(t, io.StdOut.String(), tc.out)
		})
	}
}
