<script lang="ts">
	// Spacing is the caller's business: the main page's dividers carry py-2,
	// the footer carries pt-14, the workspace page uses its own rhythm. Baking
	// a padding in would silently move whichever caller disagreed with it.
	let { class: klass = '' }: { class?: string } = $props();
</script>

<!--
	The bronze ornament: a rotated diamond flanked by fading dots. Used instead
	of a horizontal rule wherever the paper-and-ink pages mark a passage
	between blocks (ADR 2026071102).
-->
<div class="flex items-center justify-center gap-3 {klass}" aria-hidden="true">
	<span class="h-1 w-1 rounded-full bg-[#b08d57]/40"></span>
	<span class="h-1.5 w-1.5 rounded-full bg-[#b08d57]/60"></span>
	<span class="inline-block h-2.5 w-2.5 rotate-45 bg-[#b08d57]"></span>
	<span class="h-1.5 w-1.5 rounded-full bg-[#b08d57]/60"></span>
	<span class="h-1 w-1 rounded-full bg-[#b08d57]/40"></span>
</div>
