package main

import (
	sharedtesters "github.com/chendingplano/shared/go/api/testers"
)

// registerAll registers all shared and application-specific testers.
func registerAll() {
	// Shared library testers
	sharedtesters.RegisterTesters()

	/*
		// Application-specific testers
		autotester.GlobalRegistry.Register("user_tester", func() autotester.Tester {
			return apptests.NewUserTester(cfg)
		})
		autotester.GlobalRegistry.Register("project_tester", func() autotester.Tester {
			return apptests.NewProjectTester(cfg)
		})
		autotester.GlobalRegistry.Register("document_tester", func() autotester.Tester {
			return apptests.NewDocumentTester(cfg)
		})
		autotester.GlobalRegistry.Register("email_tester", func() autotester.Tester {
			return apptests.NewEmailTester(cfg)
		})
	*/
}
