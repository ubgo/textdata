package aws_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugins/aws"
)

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
	doc, err := jsonc.Parse([]byte(`{"db": {"url": "${aws:ssm//app/prod/db_url}"}}`), jsonc.WithPlugin(p))
	if err != nil {
		fmt.Println(err)
		return
	}
	v, err := doc.GetExpanded("db.url")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v)
	fmt.Println(string(doc.Render()))
	// Output:
	// postgres://db/app
	// {"db": {"url": "${aws:ssm//app/prod/db_url}"}}
}
