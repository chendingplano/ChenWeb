// Command create-admin bootstraps the default "sysadmin" account in Kratos
// when deploying ChenWeb (AUTH_USE_KRATOS=true). Safe to re-run: if an
// identity with the given email already exists, it is simply promoted to
// admin instead of being recreated.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/auth"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

func generatePassword() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("failed to generate random password: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func main() {
	email := flag.String("email", "", "Email for the sysadmin account (required)")
	firstName := flag.String("first-name", "sysadmin", "First name / display name for the sysadmin account")
	lastName := flag.String("last-name", "", "Last name for the sysadmin account")
	password := flag.String("password", "", "Password for the sysadmin account (auto-generated and printed once if omitted)")
	flag.Parse()

	_ = godotenv.Load("./.env")

	logger := loggerutil.CreateDefaultLogger("CWB_CREATE_ADMIN_001")
	defer logger.Close()

	if strings.TrimSpace(*email) == "" {
		fmt.Fprintln(os.Stderr, "error: -email is required (Kratos identities are keyed by email)")
		os.Exit(1)
	}

	generatedPassword := false
	if strings.TrimSpace(*password) == "" {
		*password = generatePassword()
		generatedPassword = true
	}

	existing, err := auth.KratosGetIdentityByEmail(logger, *email)
	if err == nil && existing != nil {
		if promoteErr := auth.KratosUpdateIdentity(logger, existing.UserId, auth.KratosIdentityUpdate{
			MetadataPublic: map[string]interface{}{
				"admin": true,
				"roles": []string{"admin"},
			},
		}); promoteErr != nil {
			fmt.Fprintf(os.Stderr, "error: identity already exists but could not be promoted to admin: %v\n", promoteErr)
			os.Exit(1)
		}
		fmt.Printf("sysadmin account already existed (email=%s, id=%s); ensured admin=true\n", *email, existing.UserId)
		return
	}
	if err != nil && !strings.Contains(err.Error(), "identity not found for email") {
		fmt.Fprintf(os.Stderr, "error: failed to look up existing identity: %v\n", err)
		os.Exit(1)
	}

	created, err := auth.KratosCreateIdentityWithPassword(logger, *email, *firstName, *lastName, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create sysadmin identity: %v\n", err)
		os.Exit(1)
	}

	if err := auth.KratosUpdateIdentity(logger, created.UserId, auth.KratosIdentityUpdate{
		MetadataPublic: map[string]interface{}{
			"admin": true,
			"roles": []string{"admin"},
		},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "error: identity created but failed to grant admin: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created sysadmin account (email=%s, id=%s)\n", *email, created.UserId)
	if generatedPassword {
		fmt.Printf("generated password: %s\n", *password)
		fmt.Println("store this securely and change it after first login; it will not be shown again.")
	}
}
