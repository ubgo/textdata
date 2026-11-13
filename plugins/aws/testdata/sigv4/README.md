# SigV4 test vectors

These directories are the AWS Signature Version 4 test suite as published in `github.com/awslabs/aws-c-auth` (`tests/aws-signing-test-suite/v4`), Apache License 2.0. Only the header-signing files are kept (`context.json`, `request.txt`, `header-canonical-request.txt`, `header-string-to-sign.txt`, `header-signature.txt`, `header-signed-request.txt`); the query-signing (presigned URL) variants are not used because the plugin never presigns. The credentials inside are AWS's documented example credentials, not real ones.

`sigv4_test.go` signs every `request.txt` and compares the canonical request, the string to sign, the signature and the `Authorization` header byte for byte.
