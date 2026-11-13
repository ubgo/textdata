package aws_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/ubgo/textdata/plugins/aws"
)

// The plugin is a base Resolver, so a format calls it for ${aws:ssm/…} and
// ${aws:sm/…}. Here it is called directly; the integration module shows the
// same instance installed in dotenv and jsonc.
func Example() {
	// A stand-in for Parameter Store answering GetParameter.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Parameter":{"Name":"/app/prod/db_url","Value":"postgres://db/app"}}`)
	}))
	defer srv.Close()

	p, err := aws.New(
		aws.WithRegion("eu-west-1"),
		aws.WithCredentials(aws.Credentials{AccessKeyID: "EXAMPLEID", SecretAccessKey: "example-secret"}),
		aws.WithEndpoint(srv.URL), // omit to talk to AWS itself
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	v, err := p.Resolve("aws", "ssm//app/prod/db_url") // ${aws:ssm//app/prod/db_url}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v)
	// Output:
	// postgres://db/app
}
