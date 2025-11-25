<!-- App.svelte -->
<script>
  import PromptStore from '$lib/components/prompt-store/PromptStore.svelte';
  
  // Mock data for demonstration
  const mockPrompts = [
    {
      PromptID: 1,
      PromptName: "creative_writing",
      PromptDesc: "Creative writing assistant prompt",
      PromptContent: "You are a creative writing assistant...",
      Status: "Active",
      PromptPurpose: "Writing",
      PromptSource: "User Entered",
      PromptKeywords: "writing,creative,assistant",
      PromptTags: "writing,creative",
      Creator: "user1",
      Updater: "user1",
      CreatorLoc: "web-ui",
      UpdaterLoc: "web-ui",
      CreatedAt: "2024-01-15T10:30:00Z",
      UpdatedAt: "2024-01-15T10:30:00Z"
    },
    {
      PromptID: 2,
      PromptName: "code_review",
      PromptDesc: "Code review assistant prompt",
      PromptContent: "You are a code review expert...",
      Status: "Active",
      PromptPurpose: "Programming",
      PromptSource: "Upload",
      PromptKeywords: "code,review,programming",
      PromptTags: "programming,code",
      Creator: "user2",
      Updater: "user2",
      CreatorLoc: "api",
      UpdaterLoc: "api",
      CreatedAt: "2024-01-16T14:20:00Z",
      UpdatedAt: "2024-01-16T14:20:00Z"
    }
  ];
</script>

<main>
  <h1>Prompt Store Management</h1>
  <PromptStore/>
</main>

<style>
  main {
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px;
    font-family: Arial, sans-serif;
  }
  
  h1 {
    color: #333;
    margin-bottom: 30px;
    text-align: center;
  }
</style>