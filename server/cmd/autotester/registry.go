package main

import (
	autotester "github.com/chendingplano/shared/go/api/autotesters"
	sharedtesters "github.com/chendingplano/shared/go/api/testers"
	"github.com/dinglind/mirai/server/cmd/config"
)

// registerAll registers all shared and application-specific testers.
func registerAll(_ *config.Config) {
	// Shared library testers
	autotester.GlobalRegistry.Register("tester_database", func() autotester.Tester {
		return sharedtesters.NewDatabaseTester(&config.PGConfig)
	})
	autotester.GlobalRegistry.Register("tester_databaseutil", func() autotester.Tester {
		return sharedtesters.NewDatabaseUtilTester()
	})
	autotester.GlobalRegistry.Register("tester_logger", func() autotester.Tester {
		return sharedtesters.NewLoggerTester()
	})

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
