package bitbucket

import (
	"encoding/json"
	"testing"
)

// Bodies below are trimmed from real api.bitbucket.org responses.

func TestRepoDecodesTheFieldsBitbucketRepositoryDoesNotModel(t *testing.T) {
	const body = `{"values":[{"type":"repository","full_name":"acme/widget","name":"widget",
	  "slug":"widget","uuid":"{8c7ebd36}","is_private":true,"fork_policy":"no_public_forks",
	  "created_on":"2026-09-16T07:10:14.170832+00:00","updated_on":"2026-09-16T07:10:18.288204+00:00",
	  "size":2303,"scm":"git","language":"","mainbranch":{"name":"dev","type":"branch"},
	  "project":{"key":"KON","name":"konfirmity","uuid":"{3ae2805c}"},
	  "workspace":{"slug":"acme","name":"acme","uuid":"{751f8ab2}"},
	  "owner":{"display_name":"acme","uuid":"{751f8ab2}","type":"team"},
	  "links":{"self":{"href":"https://api.bitbucket.org/2.0/repositories/acme/widget"}}}],
	  "next":"https://api.bitbucket.org/2.0/repositories/acme?page=2"}`

	list := new(RepoList)
	if err := json.Unmarshal([]byte(body), list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Repositories) != 1 {
		t.Fatalf("got %d repositories, want 1", len(list.Repositories))
	}
	repo := list.Repositories[0]

	if repo.Full_name != "acme/widget" || !repo.Is_private {
		t.Errorf("full_name/is_private did not decode: %+v", repo)
	}
	// The workspace slug is how a row is attributed back to the workspace it was listed under.
	if repo.Workspace.Slug != "acme" {
		t.Errorf("workspace slug = %q, want acme", repo.Workspace.Slug)
	}
	// These four are absent from bitbucket.Repository, which is why the table carries its own type.
	if repo.CreatedOn == nil || repo.UpdatedOn == nil {
		t.Errorf("created_on/updated_on did not decode: %+v", repo)
	}
	if repo.Size != 2303 || repo.Scm != "git" {
		t.Errorf("size/scm did not decode: size=%d scm=%q", repo.Size, repo.Scm)
	}
	if repo.Project.Key != "KON" || repo.Project.Name != "konfirmity" {
		t.Errorf("project did not decode: %+v", repo.Project)
	}
	if list.Next == "" {
		t.Error("next page link did not decode, so pagination would stop after one page")
	}
}

// Bitbucket returns has_2fa_enabled as null even to a workspace owner. Decoding it into a bool
// turns "Bitbucket will not say" into "two-step verification is off" for every member.
func TestMemberKeepsAnUndisclosed2faStatusDistinctFromFalse(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *bool
	}{
		{"null", `{"values":[{"user":{"display_name":"A","has_2fa_enabled":null}}]}`, nil},
		{"absent", `{"values":[{"user":{"display_name":"A"}}]}`, nil},
		{"false", `{"values":[{"user":{"display_name":"A","has_2fa_enabled":false}}]}`, boolPtr(false)},
		{"true", `{"values":[{"user":{"display_name":"A","has_2fa_enabled":true}}]}`, boolPtr(true)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := new(MemberList)
			if err := json.Unmarshal([]byte(tc.body), list); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got := list.Members[0].User.Has2faEnabled
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("got %v, want null", *got)
			case tc.want != nil && got == nil:
				t.Errorf("got null, want %v", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("got %v, want %v", *got, *tc.want)
			}
		})
	}
}

func TestRepositoryPermissionDecodesTheGrantAndItsRepository(t *testing.T) {
	const body = `{"values":[{"type":"repository_permission","permission":"admin",
	  "repository":{"type":"repository","full_name":"acme/widget","name":"widget","uuid":"{8c7ebd36}"},
	  "user":{"display_name":"Ada","uuid":"{3a349a75}","account_id":"712020:5adf"}}]}`

	list := new(RepositoryPermissionList)
	if err := json.Unmarshal([]byte(body), list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p := list.Permissions[0]
	if p.Permission != "admin" {
		t.Errorf("permission = %q, want admin", p.Permission)
	}
	if p.Repository.FullName != "acme/widget" {
		t.Errorf("repository full_name = %q, want acme/widget", p.Repository.FullName)
	}
	if p.User.UUID != "{3a349a75}" || p.User.AccountId != "712020:5adf" {
		t.Errorf("user did not decode: %+v", p.User)
	}
}

func TestDeployKeyDistinguishesNeverUsedFromUnused(t *testing.T) {
	const body = `{"values":[
	  {"id":1,"label":"ci","comment":"ci@acme","key":"ssh-rsa AAAA","type":"deploy_key",
	   "created_on":"2024-01-02T03:04:05.000000+00:00","last_used":null},
	  {"id":2,"label":"deploy","key":"ssh-rsa BBBB","type":"deploy_key",
	   "created_on":"2024-01-02T03:04:05.000000+00:00","last_used":"2026-09-01T00:00:00.000000+00:00"}]}`

	list := new(DeployKeyList)
	if err := json.Unmarshal([]byte(body), list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(list.Keys))
	}
	if list.Keys[0].LastUsed != nil {
		t.Error("a key that has never been used must decode as null, not as the zero time")
	}
	if list.Keys[1].LastUsed == nil {
		t.Error("last_used did not decode")
	}
	if list.Keys[0].CreatedOn == nil {
		t.Error("created_on did not decode, so a key cannot be aged for rotation")
	}
}

func TestParseRepoFullName(t *testing.T) {
	cases := []struct {
		in, owner, repo string
	}{
		{"acme/widget", "acme", "widget"},
		{"acme", "acme", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		owner, repo := parseRepoFullName(tc.in)
		if owner != tc.owner || repo != tc.repo {
			t.Errorf("parseRepoFullName(%q) = (%q, %q), want (%q, %q)", tc.in, owner, repo, tc.owner, tc.repo)
		}
	}
}

func boolPtr(b bool) *bool { return &b }
