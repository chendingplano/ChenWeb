// Shared "please log in" dialog state for the /semos section — triggered from
// the header nav (Workspace/Knowledge Base) and the hero/closing CTA buttons,
// rendered once from the /semos layout so every nested page shares one dialog.

class LoginPrompt {
	open = $state(false);

	show() {
		this.open = true;
	}

	hide() {
		this.open = false;
	}
}

export const loginPrompt = new LoginPrompt();
