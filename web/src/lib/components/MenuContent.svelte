<!-- web/src/lib/components/MenuContent.svelte -->
<script lang="ts">
  import CalculatorIcon from "@lucide/svelte/icons/calculator";
  import CalendarIcon from "@lucide/svelte/icons/calendar";
  import CreditCardIcon from "@lucide/svelte/icons/credit-card";
  import SettingsIcon from "@lucide/svelte/icons/settings";
  import SmileIcon from "@lucide/svelte/icons/smile";
  import UserIcon from "@lucide/svelte/icons/user";
  import * as Command from "$lib/components/ui/command/index.js";

  import { createEventDispatcher } from "svelte";
  const dispatch = createEventDispatcher();

  // Helper to handle clicks
  function handleSelect(value: string) {
    console.log("Selected:", value);
    dispatch("select", value); // 👈 send the identifier
  }
</script>

<Command.Root class="rounded-lg border shadow-md md:min-w-[450px] w-full">
  <Command.Input placeholder="Type a command or search..." />
  <Command.List>
    <Command.Empty>No results found.</Command.Empty>
    <Command.Group heading="Suggestions">
      <Command.Item onclick={() => handleSelect('documents')}>
        <CalendarIcon class="mr-2 h-4 w-4" />
        <span>Documents</span>
      </Command.Item>
      <Command.Item onclick={() => handleSelect('table-03')}>
        <SmileIcon class="mr-2 h-4 w-4" />
        <span>Table 03</span>
      </Command.Item>
      <Command.Item onclick={() => handleSelect('login')}>
        <CalculatorIcon class="mr-2 h-4 w-4" />
        <span>Login</span>
      </Command.Item>
    </Command.Group>
    <Command.Separator />
    <Command.Group heading="Settings">
      <Command.Item>
        <UserIcon class="mr-2 h-4 w-4" />
        <span>Profile</span>
        <Command.Shortcut>⌘P</Command.Shortcut>
      </Command.Item>
      <Command.Item>
        <CreditCardIcon class="mr-2 h-4 w-4" />
        <span>Billing</span>
        <Command.Shortcut>⌘B</Command.Shortcut>
      </Command.Item>
      <Command.Item>
        <SettingsIcon class="mr-2 h-4 w-4" />
        <span>Settings</span>
        <Command.Shortcut>⌘S</Command.Shortcut>
      </Command.Item>
    </Command.Group>
  </Command.List>
</Command.Root>