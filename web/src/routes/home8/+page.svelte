<script lang="ts">
	import Heart from '@lucide/svelte/icons/heart';
	import ShoppingBag from '@lucide/svelte/icons/shopping-bag';
	import Search from '@lucide/svelte/icons/search';
	import Plus from '@lucide/svelte/icons/plus';
	import Minus from '@lucide/svelte/icons/minus';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Pencil from '@lucide/svelte/icons/pencil';
	import X from '@lucide/svelte/icons/x';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import ArrowRight from '@lucide/svelte/icons/arrow-right';
	import Sparkles from '@lucide/svelte/icons/sparkles';
	import Truck from '@lucide/svelte/icons/truck';
	import Scissors from '@lucide/svelte/icons/scissors';
	import Mail from '@lucide/svelte/icons/mail';
	import Check from '@lucide/svelte/icons/check';
	import Instagram from '@lucide/svelte/icons/instagram';
	import Leaf from '@lucide/svelte/icons/leaf';
	import Ruler from '@lucide/svelte/icons/ruler';
	import Clock from '@lucide/svelte/icons/clock';
	import BadgeCheck from '@lucide/svelte/icons/badge-check';
	import Layers from '@lucide/svelte/icons/layers';

	// ── Types ───────────────────────────────────────────────────────────────
	type Colorway = { base: string; yarn: string; accent: string };
	type Item = {
		id: string;
		name: string;
		price: number;
		category: string;
		maker: string;
		thirdParty: boolean;
		rating: number;
		reviewCount: number;
		description: string;
		materials: string[];
		dimensions: string;
		processing: string;
		colorway: Colorway;
		badge?: string;
		photo?: string;
	};
	type Review = { id: string; itemId: string; name: string; rating: number; date: string; text: string };
	type Difficulty = 'Beginner' | 'Intermediate' | 'Advanced';
	type Status = 'Published' | 'Testing' | 'Draft';
	type Pattern = {
		id: string;
		name: string;
		difficulty: Difficulty;
		hook: string;
		yarnWeight: string;
		status: Status;
		estTime: string;
		notes: string;
	};

	// ── Colorways (hand-named, like real yarn) ──────────────────────────────
	const cw = {
		marigold: { base: 'oklch(0.93 0.06 88)', yarn: 'oklch(0.80 0.13 78)', accent: 'oklch(0.62 0.15 55)' },
		raspberry: { base: 'oklch(0.92 0.045 20)', yarn: 'oklch(0.70 0.16 16)', accent: 'oklch(0.54 0.18 12)' },
		sage: { base: 'oklch(0.93 0.03 150)', yarn: 'oklch(0.76 0.07 155)', accent: 'oklch(0.56 0.07 160)' },
		dustyrose: { base: 'oklch(0.92 0.035 12)', yarn: 'oklch(0.79 0.08 8)', accent: 'oklch(0.60 0.10 6)' },
		cornflower: { base: 'oklch(0.92 0.04 250)', yarn: 'oklch(0.72 0.10 255)', accent: 'oklch(0.55 0.12 260)' },
		terracotta: { base: 'oklch(0.90 0.05 50)', yarn: 'oklch(0.68 0.13 45)', accent: 'oklch(0.52 0.13 38)' },
		lavender: { base: 'oklch(0.92 0.04 300)', yarn: 'oklch(0.75 0.09 300)', accent: 'oklch(0.58 0.12 305)' },
		seafoam: { base: 'oklch(0.93 0.04 190)', yarn: 'oklch(0.78 0.08 190)', accent: 'oklch(0.60 0.09 195)' },
		mustard: { base: 'oklch(0.92 0.06 95)', yarn: 'oklch(0.78 0.12 92)', accent: 'oklch(0.60 0.12 80)' },
		plum: { base: 'oklch(0.88 0.05 330)', yarn: 'oklch(0.62 0.13 340)', accent: 'oklch(0.48 0.14 345)' },
		sky: { base: 'oklch(0.93 0.04 230)', yarn: 'oklch(0.78 0.09 230)', accent: 'oklch(0.60 0.11 235)' },
		oatmeal: { base: 'oklch(0.93 0.015 80)', yarn: 'oklch(0.82 0.03 78)', accent: 'oklch(0.66 0.04 70)' }
	} satisfies Record<string, Colorway>;

	// ── Catalog ─────────────────────────────────────────────────────────────
	const items: Item[] = [
		{
			id: 'cloud-bunny', name: 'Cloud Bunny Amigurumi', price: 34, category: 'Amigurumi',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.9, reviewCount: 218, colorway: cw.dustyrose,
			badge: 'Bestseller',
			description: 'A palm-sized bunny crocheted in the round from butter-soft cotton, with a weighted bottom so it sits up on a shelf and tiny embroidered eyes that will never come loose. Each one takes an evening to make, so no two are exactly alike.',
			materials: ['100% combed cotton', 'Polyester fiberfill', 'Safety-free embroidered eyes'],
			dimensions: '14 cm tall, 9 cm wide', processing: 'Made to order, ships in 3 to 5 days'
		},
		{
			id: 'granny-throw', name: 'Granny Square Throw Blanket', price: 128, category: 'Home',
			maker: 'Jenny Gu', thirdParty: false, rating: 5.0, reviewCount: 96, colorway: cw.marigold,
			badge: 'Star Seller pick',
			description: 'Forty-nine hand-joined granny squares in a sunrise gradient, bordered with a scalloped edge. Heavy enough to be the blanket everyone fights over on the couch, light enough to carry to the porch.',
			materials: ['Worsted acrylic and wool blend', 'Hand-blocked, machine washable cold'],
			dimensions: '120 cm by 150 cm', processing: 'Made to order, ships in 2 to 3 weeks'
		},
		{
			id: 'market-tote', name: 'Mesh Market Tote', price: 46, category: 'Bags',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.7, reviewCount: 142, colorway: cw.sage,
			description: 'An open-weave cotton tote that packs flat and stretches to swallow a farmers-market haul. Reinforced base and shoulder-length handles that hold up to a watermelon, tested personally.',
			materials: ['100% cotton, undyed and dyed lots', 'Reinforced double-crochet base'],
			dimensions: '38 cm by 40 cm, handles 30 cm', processing: 'Made to order, ships in 4 to 6 days'
		},
		{
			id: 'mushroom-purse', name: 'Toadstool Coin Purse', price: 18, category: 'Accessories',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.8, reviewCount: 173, colorway: cw.raspberry,
			description: 'A little red toadstool with white bobble spots and a brass zip tucked under the cap. Big enough for earbuds, coins, and a folded note. The pocketable bit of whimsy people end up buying three of.',
			materials: ['Cotton blend', 'Brass zipper', 'Cotton lining'],
			dimensions: '9 cm by 8 cm', processing: 'Ready to ship in 1 to 2 days'
		},
		{
			id: 'bobble-beanie', name: 'Chunky Bobble Beanie', price: 32, category: 'Wearables',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.6, reviewCount: 88, colorway: cw.terracotta,
			description: 'A squishy ribbed beanie worked in chunky merino with a removable faux-fur pompom. Folds into a deep brim for cold mornings and stretches to fit most adult heads without losing its shape.',
			materials: ['Chunky merino wool', 'Removable pompom'],
			dimensions: 'Fits 54 to 60 cm head', processing: 'Made to order, ships in 5 to 7 days'
		},
		{
			id: 'strawberry-booties', name: 'Strawberry Baby Booties', price: 24, category: 'Baby',
			maker: 'Jenny Gu', thirdParty: false, rating: 5.0, reviewCount: 134, colorway: cw.raspberry,
			badge: 'Baby favorite',
			description: 'Soft strawberry booties with a tiny green leaf cuff and a non-slip sole, sized for the first six months. The most-gifted thing in the shop, and the reason for a good number of baby-shower tears.',
			materials: ['Organic cotton', 'Hypoallergenic fill in the toe'],
			dimensions: '0 to 6 months, 9 cm sole', processing: 'Made to order, ships in 3 to 5 days'
		},
		{
			id: 'plant-hammock', name: 'Hanging Plant Hammock', price: 28, category: 'Home',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.5, reviewCount: 61, colorway: cw.seafoam,
			description: 'A macrame-style crochet sling that cradles a 4 to 6 inch pot and hangs your trailing pothos in the window where it belongs. Adjustable wood bead lets you level a wobbly plant.',
			materials: ['Natural cotton cord', 'Wooden ring and beads'],
			dimensions: 'Drop 80 cm, holds a 15 cm pot', processing: 'Ready to ship in 1 to 2 days'
		},
		{
			id: 'daisy-coasters', name: 'Daisy Coaster Set of Four', price: 22, category: 'Home',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.8, reviewCount: 110, colorway: cw.mustard,
			description: 'Four crocheted daisies stiffened to hold their shape and thirsty enough to catch a sweating glass of iced tea. Sold as a set of four, because nobody hosts for one.',
			materials: ['Cotton, fabric-stiffened', 'Wipe clean'],
			dimensions: '11 cm across each', processing: 'Ready to ship in 1 to 2 days'
		},
		{
			id: 'highland-cow', name: 'Highland Cow Plushie', price: 42, category: 'Amigurumi',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.9, reviewCount: 79, colorway: cw.oatmeal,
			badge: 'New',
			description: 'A shaggy highland cow with a fringe you can barely see his eyes through and little felt horns. The brushed-out loop-stitch coat takes hours, and it shows. He is unreasonably huggable.',
			materials: ['Brushed acrylic, loop-stitch coat', 'Felt horns and ears'],
			dimensions: '20 cm tall', processing: 'Made to order, ships in 1 to 2 weeks'
		},
		{
			id: 'scallop-top', name: 'Scalloped Summer Top', price: 78, category: 'Wearables',
			maker: 'Jenny Gu', thirdParty: false, rating: 4.7, reviewCount: 53, colorway: cw.cornflower,
			description: 'A breezy open-stitch top with a scalloped hem, made to layer over a slip on hot days. Worked seamlessly from the top down so it drapes instead of boxing you in. Made to your measurements.',
			materials: ['Cotton and linen blend', 'Hand-finished neckline'],
			dimensions: 'Made to measure, XS to XXL', processing: 'Made to order, ships in 2 to 3 weeks'
		},
		// Guest makers shown in the gallery
		{
			id: 'stoneware-mug', name: 'Speckled Stoneware Mug', price: 26, category: 'Ceramics',
			maker: 'Kiln & Clay Co.', thirdParty: true, rating: 4.8, reviewCount: 64, colorway: cw.oatmeal,
			description: 'A wheel-thrown mug with a thumb rest and a glaze that pools darker in the speckles. Holds a generous twelve ounces and keeps coffee warm a little longer than it has any right to.',
			materials: ['Stoneware clay', 'Food-safe reactive glaze'],
			dimensions: '350 ml, 9 cm tall', processing: 'Ships in 1 week'
		},
		{
			id: 'macrame-hanging', name: 'Macrame Wall Hanging', price: 54, category: 'Fiber Art',
			maker: 'Knots by River', thirdParty: true, rating: 4.9, reviewCount: 41, colorway: cw.terracotta,
			description: 'A layered macrame wall hanging on a piece of driftwood, knotted by hand over two days. The kind of texture a blank wall has been quietly asking for.',
			materials: ['Cotton rope', 'Driftwood dowel'],
			dimensions: '60 cm wide, 90 cm drop', processing: 'Made to order, ships in 1 to 2 weeks'
		},
		{
			id: 'beeswax-candle', name: 'Hand-poured Beeswax Candle', price: 19, category: 'Home',
			maker: 'Ember Hollow', thirdParty: true, rating: 4.7, reviewCount: 88, colorway: cw.mustard,
			description: 'A pure beeswax candle with a cotton wick and the faint honey smell of the real thing. Burns clean for roughly forty hours next to your reading chair.',
			materials: ['100% beeswax', 'Cotton wick'],
			dimensions: '200 g, 8 cm tall', processing: 'Ships in 3 to 5 days'
		},
		{
			id: 'linen-napkins', name: 'Block-printed Linen Napkins', price: 38, category: 'Home',
			maker: 'Indigo Field', thirdParty: true, rating: 4.8, reviewCount: 37, colorway: cw.cornflower,
			description: 'A set of four softened-linen napkins, each hand block-printed with a botanical motif so no two prints land quite the same. They get better with every wash.',
			materials: ['Washed linen', 'Water-based ink'],
			dimensions: 'Set of 4, 40 cm square', processing: 'Ships in 1 week'
		},
		{
			id: 'hook-set', name: 'Hand-turned Crochet Hook Set', price: 44, category: 'Supplies',
			maker: 'Maple & Make', thirdParty: true, rating: 5.0, reviewCount: 52, colorway: cw.sage,
			badge: 'Maker pick',
			description: 'Five maple crochet hooks turned and oiled by hand, sizes 3.5 to 6 mm, in a rolled canvas case. Jenny crochets her samples with this exact set, which is the only review that matters.',
			materials: ['Hard maple, beeswax finish', 'Canvas roll'],
			dimensions: 'Sizes 3.5 to 6.0 mm', processing: 'Ships in 3 to 5 days'
		},
		{
			id: 'enamel-pins', name: 'Botanical Enamel Pins', price: 14, category: 'Accessories',
			maker: 'Fern & Foxglove', thirdParty: true, rating: 4.6, reviewCount: 73, colorway: cw.plum,
			description: 'A trio of hard-enamel pins, a fern, a foxglove, and a single mushroom, with rubber backs that actually stay on your bag. Small, cheap, and absurdly giftable.',
			materials: ['Hard enamel', 'Rubber clutch backs'],
			dimensions: 'Set of 3, about 3 cm each', processing: 'Ships in 2 to 4 days'
		}
	];

	// ── Persistence ─────────────────────────────────────────────────────────
	function load<T>(key: string, fallback: T): T {
		try {
			const v = localStorage.getItem(key);
			return v ? (JSON.parse(v) as T) : fallback;
		} catch {
			return fallback;
		}
	}

	const seedReviews: Review[] = [
		{ id: 'r1', itemId: 'cloud-bunny', name: 'Marisol P.', rating: 5, date: 'May 2026', text: 'Bought one for my niece and immediately ordered a second for myself. The stitches are so even it looks machine-made, but the little weighted bottom is a detail no factory bothers with.' },
		{ id: 'r2', itemId: 'cloud-bunny', name: 'Devon R.', rating: 5, date: 'Apr 2026', text: 'Arrived faster than expected and wrapped in tissue with a handwritten note. You can tell it was made by a person who cares.' },
		{ id: 'r3', itemId: 'cloud-bunny', name: 'Aiko T.', rating: 4, date: 'Apr 2026', text: 'Adorable and very soft. Slightly smaller than I pictured, but honestly that makes it cuter on a shelf.' },
		{ id: 'r4', itemId: 'granny-throw', name: 'Helen W.', rating: 5, date: 'May 2026', text: 'The gradient is even prettier in person. It is the blanket everyone reaches for first. Worth every day of the wait.' },
		{ id: 'r5', itemId: 'strawberry-booties', name: 'Priya M.', rating: 5, date: 'May 2026', text: 'Gifted these at a baby shower and the whole room gasped. The non-slip sole is a thoughtful touch for new walkers.' },
		{ id: 'r6', itemId: 'highland-cow', name: 'Tom B.', rating: 5, date: 'Apr 2026', text: 'The fringe is ridiculous in the best way. My dog is jealous of the attention it gets.' },
		{ id: 'r7', itemId: 'market-tote', name: 'Lena K.', rating: 4, date: 'Mar 2026', text: 'Stretches to fit an alarming amount of groceries and folds into nothing. Wish it came in more colors, hint hint.' },
		{ id: 'r8', itemId: 'hook-set', name: 'Sam D.', rating: 5, date: 'May 2026', text: 'These hooks are a joy to work with. Warm in the hand and the canvas roll keeps them from rolling off the table.' }
	];

	const seedPatterns: Pattern[] = [
		{ id: 'p1', name: 'Cloud Bunny Amigurumi', difficulty: 'Beginner', hook: '3.5 mm', yarnWeight: 'Worsted', status: 'Published', estTime: '1 evening', notes: 'Sells best as a finished item. Pattern PDF includes a left-handed photo set.' },
		{ id: 'p2', name: 'Sunrise Granny Square', difficulty: 'Intermediate', hook: '5.0 mm', yarnWeight: 'Aran', status: 'Published', estTime: '15 min per square', notes: 'Joined with a continuous flat-braid join. Gradient chart on page 4.' },
		{ id: 'p3', name: 'Highland Cow Plushie', difficulty: 'Intermediate', hook: '3.0 mm', yarnWeight: 'DK', status: 'Testing', estTime: '6 to 8 hours', notes: 'Two testers flagged the horn placement. Revising row 22 before release.' },
		{ id: 'p4', name: 'Scalloped Summer Top', difficulty: 'Advanced', hook: '4.0 mm', yarnWeight: 'Sport', status: 'Draft', estTime: '12 to 16 hours', notes: 'Need to grade for XXL. Add a schematic before publishing.' },
		{ id: 'p5', name: 'Daisy Coaster', difficulty: 'Beginner', hook: '4.0 mm', yarnWeight: 'Cotton', status: 'Published', estTime: '20 min each', notes: 'Great free pattern to grow the mailing list. Stiffen with fabric stiffener, not sugar water.' }
	];

	// ── State ───────────────────────────────────────────────────────────────
	let view = $state<'home' | 'shop' | 'gallery' | 'product' | 'admin'>('home');
	let selectedId = $state<string | null>(null);
	let query = $state('');
	let shopCategory = $state('All');
	let shopSort = $state('featured');
	let galleryFilter = $state<'All' | 'Jenny' | 'Guests'>('All');
	let activeImage = $state(0);
	let qty = $state(1);
	let cartOpen = $state(false);
	let toast = $state('');
	let toastTimer: ReturnType<typeof setTimeout> | undefined;

	let favorites = $state<string[]>(load<string[]>('jg_favorites', []));
	let cart = $state<{ id: string; qty: number }[]>(load('jg_cart', []));
	let reviews = $state<Review[]>(load('jg_reviews', seedReviews));
	let patterns = $state<Pattern[]>(load('jg_patterns', seedPatterns));

	// review form
	let rvName = $state('');
	let rvRating = $state(5);
	let rvText = $state('');

	// admin form
	let showForm = $state(false);
	let editingId = $state<string | null>(null);
	let adminQuery = $state('');
	let fName = $state('');
	let fDifficulty = $state<Difficulty>('Beginner');
	let fHook = $state('4.0 mm');
	let fYarn = $state('Worsted');
	let fStatus = $state<Status>('Draft');
	let fTime = $state('');
	let fNotes = $state('');

	$effect(() => { try { localStorage.setItem('jg_favorites', JSON.stringify(favorites)); } catch {} });
	$effect(() => { try { localStorage.setItem('jg_cart', JSON.stringify(cart)); } catch {} });
	$effect(() => { try { localStorage.setItem('jg_reviews', JSON.stringify(reviews)); } catch {} });
	$effect(() => { try { localStorage.setItem('jg_patterns', JSON.stringify(patterns)); } catch {} });

	// ── Derived ─────────────────────────────────────────────────────────────
	const jennyItems = items.filter((i) => !i.thirdParty);
	const categories = ['All', ...Array.from(new Set(jennyItems.map((i) => i.category)))];
	const selected = $derived(items.find((i) => i.id === selectedId) ?? null);
	const itemReviews = $derived(reviews.filter((r) => r.itemId === selectedId));
	const cartCount = $derived(cart.reduce((n, c) => n + c.qty, 0));
	const cartLines = $derived(
		cart
			.map((c) => {
				const it = items.find((i) => i.id === c.id);
				return it ? { item: it, qty: c.qty } : null;
			})
			.filter((x): x is { item: Item; qty: number } => x !== null)
	);
	const cartTotal = $derived(cartLines.reduce((n, l) => n + l.item.price * l.qty, 0));

	const shopList = $derived.by(() => {
		let list = jennyItems.slice();
		if (shopCategory !== 'All') list = list.filter((i) => i.category === shopCategory);
		const q = query.trim().toLowerCase();
		if (q) list = list.filter((i) => (i.name + ' ' + i.category + ' ' + i.description).toLowerCase().includes(q));
		if (shopSort === 'price-asc') list.sort((a, b) => a.price - b.price);
		else if (shopSort === 'price-desc') list.sort((a, b) => b.price - a.price);
		else if (shopSort === 'rating') list.sort((a, b) => b.rating - a.rating);
		return list;
	});

	const galleryList = $derived.by(() => {
		if (galleryFilter === 'Jenny') return items.filter((i) => !i.thirdParty);
		if (galleryFilter === 'Guests') return items.filter((i) => i.thirdParty);
		return items;
	});

	const related = $derived.by(() => {
		if (!selected) return [];
		return items.filter((i) => i.id !== selected.id && i.category === selected.category).slice(0, 4);
	});

	const patternList = $derived.by(() => {
		const q = adminQuery.trim().toLowerCase();
		return q ? patterns.filter((p) => (p.name + ' ' + p.notes).toLowerCase().includes(q)) : patterns;
	});
	const statusCounts = $derived({
		Published: patterns.filter((p) => p.status === 'Published').length,
		Testing: patterns.filter((p) => p.status === 'Testing').length,
		Draft: patterns.filter((p) => p.status === 'Draft').length
	});

	// ── Stitch geometry (generative crochet texture) ────────────────────────
	const STITCH = (() => {
		const W = 400, amp = 16, gap = 27, step = 36;
		const rows: { d: string }[] = [];
		for (let y = 6; y < 340; y += gap) {
			let d = `M -12 ${y}`;
			for (let x = -12; x <= W + step; x += step) {
				d += ` L ${x + step / 2} ${y - amp} L ${x + step} ${y}`;
			}
			rows.push({ d });
		}
		return rows;
	})();

	// ── Actions ─────────────────────────────────────────────────────────────
	function go(v: typeof view) {
		view = v;
		cartOpen = false;
		if (typeof window !== 'undefined') window.scrollTo({ top: 0 });
	}
	function openProduct(id: string) {
		selectedId = id;
		activeImage = 0;
		qty = 1;
		go('product');
	}
	function toggleFav(id: string) {
		favorites = favorites.includes(id) ? favorites.filter((f) => f !== id) : [...favorites, id];
	}
	function addToCart(id: string, n = 1) {
		const line = cart.find((c) => c.id === id);
		if (line) cart = cart.map((c) => (c.id === id ? { ...c, qty: c.qty + n } : c));
		else cart = [...cart, { id, qty: n }];
		const it = items.find((i) => i.id === id);
		showToast(`Added ${it?.name ?? 'item'} to your basket`);
	}
	function setLineQty(id: string, n: number) {
		if (n <= 0) cart = cart.filter((c) => c.id !== id);
		else cart = cart.map((c) => (c.id === id ? { ...c, qty: n } : c));
	}
	function showToast(msg: string) {
		toast = msg;
		clearTimeout(toastTimer);
		toastTimer = setTimeout(() => (toast = ''), 2400);
	}
	function submitSearch(e: Event) {
		e.preventDefault();
		shopCategory = 'All';
		go('shop');
	}
	function cardKey(e: KeyboardEvent, id: string) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			openProduct(id);
		}
	}
	function onPhotoError(e: Event) {
		(e.currentTarget as HTMLImageElement).style.opacity = '0';
	}

	function addReview(e: Event) {
		e.preventDefault();
		if (!selected || !rvName.trim() || !rvText.trim()) return;
		reviews = [
			{
				id: 'u' + Date.now(),
				itemId: selected.id,
				name: rvName.trim(),
				rating: rvRating,
				date: 'Jun 2026',
				text: rvText.trim()
			},
			...reviews
		];
		rvName = '';
		rvText = '';
		rvRating = 5;
		showToast('Thank you, your review is posted');
	}

	function ratingBreakdown(list: Review[]) {
		const counts = [0, 0, 0, 0, 0];
		for (const r of list) counts[5 - r.rating]++;
		const total = list.length || 1;
		return counts.map((c, i) => ({ star: 5 - i, count: c, pct: Math.round((c / total) * 100) }));
	}

	// admin
	function openNew() {
		editingId = null;
		fName = '';
		fDifficulty = 'Beginner';
		fHook = '4.0 mm';
		fYarn = 'Worsted';
		fStatus = 'Draft';
		fTime = '';
		fNotes = '';
		showForm = true;
	}
	function openEdit(p: Pattern) {
		editingId = p.id;
		fName = p.name;
		fDifficulty = p.difficulty;
		fHook = p.hook;
		fYarn = p.yarnWeight;
		fStatus = p.status;
		fTime = p.estTime;
		fNotes = p.notes;
		showForm = true;
	}
	function savePattern(e: Event) {
		e.preventDefault();
		if (!fName.trim()) return;
		const data: Pattern = {
			id: editingId ?? 'p' + Date.now(),
			name: fName.trim(),
			difficulty: fDifficulty,
			hook: fHook.trim(),
			yarnWeight: fYarn.trim(),
			status: fStatus,
			estTime: fTime.trim() || 'Not set',
			notes: fNotes.trim()
		};
		if (editingId) patterns = patterns.map((p) => (p.id === editingId ? data : p));
		else patterns = [data, ...patterns];
		showForm = false;
		showToast(editingId ? 'Pattern updated' : 'Pattern added');
	}
	function deletePattern(id: string) {
		patterns = patterns.filter((p) => p.id !== id);
		if (editingId === id) showForm = false;
		showToast('Pattern removed');
	}
</script>

<svelte:head>
	<title>Jenny Gu — Handmade Crochet &amp; Small-batch Craft</title>
	<meta
		name="description"
		content="Crochet made one stitch at a time by Jenny Gu, plus a gallery of guest makers. Amigurumi, blankets, bags, and wearables, all handmade to order."
	/>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,400;12..96,600;12..96,800&family=Hanken+Grotesk:wght@400;500;600;700&family=Caveat:wght@600;700&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<!-- ── Reusable snippets ──────────────────────────────────────────────── -->
{#snippet swatch(c: Colorway, seed: string)}
	<svg class="swatch" viewBox="0 0 400 320" preserveAspectRatio="xMidYMid slice" role="presentation">
		<defs>
			<radialGradient id={'sheen-' + seed} cx="32%" cy="22%" r="85%">
				<stop offset="0%" stop-color="white" stop-opacity="0.28" />
				<stop offset="55%" stop-color="white" stop-opacity="0" />
				<stop offset="100%" stop-color="black" stop-opacity="0.10" />
			</radialGradient>
		</defs>
		<rect x="0" y="0" width="400" height="320" fill={c.base} />
		{#each STITCH as r, i}
			<path
				d={r.d}
				fill="none"
				stroke={i % 2 ? c.accent : c.yarn}
				stroke-width="12"
				stroke-linejoin="round"
				stroke-linecap="round"
				opacity="0.95"
			/>
		{/each}
		<rect x="0" y="0" width="400" height="320" fill={'url(#sheen-' + seed + ')'} />
	</svg>
{/snippet}

{#snippet stars(value: number, size = 15)}
	<span class="stars" style="--w:{Math.max(0, Math.min(100, (value / 5) * 100))}%; font-size:{size}px">
		<span class="s-base" aria-hidden="true">★★★★★</span>
		<span class="s-fill" aria-hidden="true">★★★★★</span>
		<span class="sr-only">{value} out of 5</span>
	</span>
{/snippet}

{#snippet productCard(item: Item)}
	<div
		class="card"
		role="button"
		tabindex="0"
		onclick={() => openProduct(item.id)}
		onkeydown={(e) => cardKey(e, item.id)}
	>
		<div class="card-media">
			{@render swatch(item.colorway, item.id)}
			{#if item.badge}<span class="ribbon">{item.badge}</span>{/if}
			<button
				class="fav {favorites.includes(item.id) ? 'on' : ''}"
				aria-label={favorites.includes(item.id) ? 'Remove from favorites' : 'Add to favorites'}
				onclick={(e) => {
					e.stopPropagation();
					toggleFav(item.id);
				}}
			>
				<Heart size={17} fill={favorites.includes(item.id) ? 'currentColor' : 'none'} />
			</button>
		</div>
		<div class="card-body">
			<div class="card-maker">
				{item.maker}
				{#if item.thirdParty}<span class="guest-tag">Guest maker</span>{/if}
			</div>
			<h3>{item.name}</h3>
			<div class="card-foot">
				<span class="price">${item.price}</span>
				<span class="rate">{@render stars(item.rating, 13)}<em>{item.reviewCount}</em></span>
			</div>
		</div>
	</div>
{/snippet}

<div class="app">
	<!-- ── Header ─────────────────────────────────────────────────────────── -->
	<header class="topbar">
		<button class="brand" onclick={() => go('home')} aria-label="Jenny Gu home">
			<span class="brand-mark">{@render swatch(cw.marigold, 'logo')}</span>
			<span class="brand-text">
				<span class="brand-name">Jenny Gu</span>
				<span class="brand-sub">handmade with yarn &amp; patience</span>
			</span>
		</button>

		<nav class="nav">
			<button class:active={view === 'home'} onclick={() => go('home')}>Home</button>
			<button class:active={view === 'shop'} onclick={() => go('shop')}>Shop</button>
			<button class:active={view === 'gallery'} onclick={() => go('gallery')}>Gallery</button>
			<button class:active={view === 'admin'} onclick={() => go('admin')}>Pattern Studio</button>
		</nav>

		<div class="tools">
			<form class="search" onsubmit={submitSearch}>
				<Search size={16} />
				<input
					type="search"
					placeholder="Search the shop"
					bind:value={query}
					aria-label="Search the shop"
				/>
			</form>
			<button class="icon-btn" onclick={() => go('shop')} aria-label="Favorites">
				<Heart size={19} />
				{#if favorites.length}<span class="count">{favorites.length}</span>{/if}
			</button>
			<button class="icon-btn" onclick={() => (cartOpen = true)} aria-label="Open basket">
				<ShoppingBag size={19} />
				{#if cartCount}<span class="count accent">{cartCount}</span>{/if}
			</button>
		</div>
	</header>

	{#if toast}
		<div class="toast" role="status">
			<Check size={16} />
			{toast}
		</div>
	{/if}

	<main>
		<!-- ── HOME ─────────────────────────────────────────────────────────── -->
		{#if view === 'home'}
			<section class="hero">
				<div class="hero-copy">
					<span class="kicker"><Sparkles size={14} /> Small batch, made to order in Plano, TX</span>
					<h1>Yarn, turned into things <span class="script">people keep.</span></h1>
					<p>
						I am Jenny. I crochet bunnies, blankets, and beanies one evening at a time, then send
						them off to live in other people's homes. Nothing here is mass produced, and that is
						entirely the point.
					</p>
					<div class="hero-actions">
						<button class="btn btn-primary" onclick={() => go('shop')}>Shop the collection</button>
						<button class="btn btn-ghost" onclick={() => go('gallery')}>
							Browse the gallery <ArrowRight size={16} />
						</button>
					</div>
					<div class="hero-trust">
						<span><BadgeCheck size={15} /> Star Seller</span>
						<span><Heart size={15} /> 1,300+ five-star reviews</span>
						<span><Truck size={15} /> Ships worldwide</span>
					</div>
				</div>
				<div class="hero-art">
					<div class="hero-tile big">{@render swatch(cw.marigold, 'h1')}</div>
					<div class="hero-tile">{@render swatch(cw.raspberry, 'h2')}</div>
					<div class="hero-tile">{@render swatch(cw.cornflower, 'h3')}</div>
					<div class="hero-tile wide">{@render swatch(cw.sage, 'h4')}</div>
					<span class="hero-sticker">made by hand, not by machine</span>
				</div>
			</section>

			<section class="strip">
				<div class="strip-head">
					<h2>This week's favorites</h2>
					<button class="link" onclick={() => go('shop')}>See everything <ArrowRight size={15} /></button>
				</div>
				<div class="grid">
					{#each jennyItems.slice(0, 4) as item (item.id)}
						{@render productCard(item)}
					{/each}
				</div>
			</section>

			<section class="categories">
				<h2>Find your kind of cozy</h2>
				<div class="cat-row">
					{#each ['Amigurumi', 'Home', 'Wearables', 'Baby'] as catName, i (catName)}
						<button
							class="cat"
							onclick={() => {
								shopCategory = catName;
								query = '';
								go('shop');
							}}
						>
							<div class="cat-art">
								{@render swatch([cw.dustyrose, cw.mustard, cw.cornflower, cw.raspberry][i], 'cat' + i)}
							</div>
							<span>{catName}</span>
						</button>
					{/each}
				</div>
			</section>

			<section class="about">
				<div class="about-photo">
					<div class="about-fallback">{@render swatch(cw.sage, 'about')}</div>
					<img
						src="https://images.unsplash.com/photo-1604859469887-d75520d3b9c0?auto=format&fit=crop&w=1100&q=80"
						alt="Jenny's hands working a crochet hook through cream-colored yarn at a sunlit table"
						loading="lazy"
						onerror={onPhotoError}
					/>
				</div>
				<div class="about-copy">
					<span class="kicker"><Scissors size={14} /> Meet the maker</span>
					<h2>Every piece starts as a tangle and a quiet hour.</h2>
					<p>
						I learned to crochet from my grandmother, mostly to keep my hands busy. Years later the
						hobby outgrew the shelf it was supposed to fit on, so I started selling. I still make
						everything myself, choose every colorway, and weave in every loose end by hand.
					</p>
					<p>
						When I am not making for the shop, I write crochet patterns so other people can make
						these too. You can find those in the Pattern Studio.
					</p>
					<div class="about-stats">
						<div><strong>2018</strong><span>first sale</span></div>
						<div><strong>3,400+</strong><span>orders shipped</span></div>
						<div><strong>1 pair</strong><span>of hands</span></div>
					</div>
				</div>
			</section>

			<section class="how">
				<h2>How an order becomes a thing</h2>
				<ol class="steps">
					<li>
						<span class="step-n">01</span>
						<h3>You pick a colorway</h3>
						<p>Choose from the shop, or message me to dream up something in your colors.</p>
					</li>
					<li>
						<span class="step-n">02</span>
						<h3>I make it, start to finish</h3>
						<p>One project at a time, by hand, usually with a cat supervising the process.</p>
					</li>
					<li>
						<span class="step-n">03</span>
						<h3>It ships, wrapped with a note</h3>
						<p>Tissue, a care card, and a thank you, because a person made this for a person.</p>
					</li>
				</ol>
			</section>

			<section class="news">
				<div class="news-inner">
					<div>
						<h2>New colorways, first dibs</h2>
						<p>A short letter when a new batch drops or a pattern goes live. No spam, ever.</p>
					</div>
					<form
						class="news-form"
						onsubmit={(e) => {
							e.preventDefault();
							showToast('You are on the list, thank you');
						}}
					>
						<Mail size={17} />
						<input type="email" required placeholder="you@example.com" aria-label="Email address" />
						<button class="btn btn-primary" type="submit">Join</button>
					</form>
				</div>
			</section>
		{/if}

		<!-- ── SHOP ─────────────────────────────────────────────────────────── -->
		{#if view === 'shop'}
			<section class="page-head">
				<div>
					<span class="kicker"><Layers size={14} /> The shop</span>
					<h1>Everything Jenny is making right now</h1>
					<p class="lede">
						{shopList.length} pieces, each crocheted to order. Guest makers live over in the
						<button class="inline-link" onclick={() => go('gallery')}>gallery</button>.
					</p>
				</div>
			</section>

			<div class="controls">
				<div class="chips">
					{#each categories as catName (catName)}
						<button class="chip" class:on={shopCategory === catName} onclick={() => (shopCategory = catName)}>
							{catName}
						</button>
					{/each}
				</div>
				<label class="sort">
					Sort
					<select bind:value={shopSort}>
						<option value="featured">Featured</option>
						<option value="price-asc">Price, low to high</option>
						<option value="price-desc">Price, high to low</option>
						<option value="rating">Top rated</option>
					</select>
				</label>
			</div>

			{#if shopList.length === 0}
				<div class="empty">
					<div class="empty-art">{@render swatch(cw.oatmeal, 'empty')}</div>
					<h3>Nothing matches that yet</h3>
					<p>Try another category, or clear your search.</p>
					<button
						class="btn btn-ghost"
						onclick={() => {
							query = '';
							shopCategory = 'All';
						}}>Clear filters</button
					>
				</div>
			{:else}
				<div class="grid wide-grid">
					{#each shopList as item (item.id)}
						{@render productCard(item)}
					{/each}
				</div>
			{/if}
		{/if}

		<!-- ── GALLERY ──────────────────────────────────────────────────────── -->
		{#if view === 'gallery'}
			<section class="page-head gallery-head">
				<div>
					<span class="kicker"><Sparkles size={14} /> The gallery</span>
					<h1>The full table at the craft fair</h1>
					<p class="lede">
						Jenny's crochet alongside a few guest makers she shares a booth with. Everything here is
						handmade by someone, somewhere, slowly.
					</p>
				</div>
				<div class="seg">
					<button class:on={galleryFilter === 'All'} onclick={() => (galleryFilter = 'All')}>All makers</button>
					<button class:on={galleryFilter === 'Jenny'} onclick={() => (galleryFilter = 'Jenny')}>By Jenny</button>
					<button class:on={galleryFilter === 'Guests'} onclick={() => (galleryFilter = 'Guests')}>Guest makers</button>
				</div>
			</section>

			<div class="masonry">
				{#each galleryList as item, i (item.id)}
					<div
						class="tile {i % 5 === 0 ? 'tall' : ''} {i % 7 === 3 ? 'short' : ''}"
						role="button"
						tabindex="0"
						onclick={() => openProduct(item.id)}
						onkeydown={(e) => cardKey(e, item.id)}
					>
						<div class="tile-media">
							{@render swatch(item.colorway, 'g' + item.id)}
							<button
								class="fav {favorites.includes(item.id) ? 'on' : ''}"
								aria-label="Toggle favorite"
								onclick={(e) => {
									e.stopPropagation();
									toggleFav(item.id);
								}}
							>
								<Heart size={16} fill={favorites.includes(item.id) ? 'currentColor' : 'none'} />
							</button>
							{#if item.thirdParty}<span class="guest-corner">Guest</span>{/if}
						</div>
						<div class="tile-cap">
							<div>
								<strong>{item.name}</strong>
								<span class="tile-maker">{item.maker}</span>
							</div>
							<span class="tile-price">${item.price}</span>
						</div>
					</div>
				{/each}
			</div>
		{/if}

		<!-- ── PRODUCT ──────────────────────────────────────────────────────── -->
		{#if view === 'product' && selected}
			<nav class="crumbs">
				<button onclick={() => go('home')}>Home</button>
				<span>/</span>
				<button onclick={() => (selected.thirdParty ? go('gallery') : go('shop'))}>
					{selected.thirdParty ? 'Gallery' : 'Shop'}
				</button>
				<span>/</span>
				<em>{selected.name}</em>
			</nav>

			<section class="product">
				<div class="product-media">
					<div class="main-img">
						{@render swatch(
							[selected.colorway, cw.oatmeal, cw.terracotta, cw.cornflower][activeImage] ?? selected.colorway,
							'main' + activeImage
						)}
						<button
							class="fav big {favorites.includes(selected.id) ? 'on' : ''}"
							aria-label="Toggle favorite"
							onclick={() => toggleFav(selected.id)}
						>
							<Heart size={20} fill={favorites.includes(selected.id) ? 'currentColor' : 'none'} />
						</button>
					</div>
					<div class="thumbs">
						{#each [selected.colorway, cw.oatmeal, cw.terracotta, cw.cornflower] as tc, ti}
							<button class="thumb {activeImage === ti ? 'on' : ''}" onclick={() => (activeImage = ti)} aria-label={'View angle ' + (ti + 1)}>
								{@render swatch(tc, 'thumb' + selected.id + ti)}
							</button>
						{/each}
					</div>
				</div>

				<div class="product-info">
					<div class="maker-line">
						<span>{selected.maker}</span>
						{#if !selected.thirdParty}<span class="star-seller"><BadgeCheck size={14} /> Star Seller</span>{/if}
						{#if selected.thirdParty}<span class="guest-tag">Guest maker</span>{/if}
					</div>
					<h1>{selected.name}</h1>
					<div class="prod-rate">
						{@render stars(selected.rating, 18)}
						<strong>{selected.rating.toFixed(1)}</strong>
						<button class="inline-link" onclick={() => { const el = document.getElementById('reviews'); el?.scrollIntoView({ behavior: 'smooth' }); }}>
							{selected.reviewCount} reviews
						</button>
					</div>
					<div class="price-line">
						<span class="big-price">${selected.price}</span>
						{#if !selected.thirdParty}<span class="vat">Made to order, just for you</span>{/if}
					</div>

					<p class="prod-desc">{selected.description}</p>

					<div class="buy">
						<div class="qty">
							<button onclick={() => (qty = Math.max(1, qty - 1))} aria-label="Decrease quantity"><Minus size={16} /></button>
							<span>{qty}</span>
							<button onclick={() => (qty = qty + 1)} aria-label="Increase quantity"><Plus size={16} /></button>
						</div>
						<button class="btn btn-primary big" onclick={() => addToCart(selected.id, qty)}>
							<ShoppingBag size={18} /> Add to basket
						</button>
						<button class="btn btn-outline big" onclick={() => toggleFav(selected.id)}>
							<Heart size={18} fill={favorites.includes(selected.id) ? 'currentColor' : 'none'} />
							{favorites.includes(selected.id) ? 'Saved' : 'Save'}
						</button>
					</div>

					<dl class="specs">
						<div><dt><Scissors size={15} /> Materials</dt><dd>{selected.materials.join(', ')}</dd></div>
						<div><dt><Ruler size={15} /> Size</dt><dd>{selected.dimensions}</dd></div>
						<div><dt><Clock size={15} /> Made to order</dt><dd>{selected.processing}</dd></div>
						<div><dt><Truck size={15} /> Shipping</dt><dd>Tracked worldwide, carbon-offset, gift wrap on request</dd></div>
					</dl>

					<div class="meet-maker">
						<div class="mm-avatar">{@render swatch(selected.thirdParty ? cw.plum : cw.marigold, 'mm' + selected.id)}</div>
						<div>
							<strong>{selected.maker}</strong>
							<p>
								{selected.thirdParty
									? 'A guest maker Jenny shares a booth with. Each piece is made in their own small studio.'
									: 'Designed, hooked, and finished by Jenny. Message before you order if you would like a custom colorway.'}
							</p>
						</div>
					</div>
				</div>
			</section>

			<!-- reviews -->
			<section class="reviews" id="reviews">
				<div class="reviews-head">
					<h2>What people say</h2>
				</div>
				<div class="reviews-layout">
					<aside class="rate-summary">
						<div class="big-rating">
							<strong>{selected.rating.toFixed(1)}</strong>
							{@render stars(selected.rating, 20)}
							<span>{selected.reviewCount} reviews</span>
						</div>
						<div class="bars">
							{#each ratingBreakdown(itemReviews) as b (b.star)}
								<div class="bar-row">
									<span>{b.star}★</span>
									<div class="bar"><i style="width:{b.pct}%"></i></div>
									<em>{b.count}</em>
								</div>
							{/each}
						</div>
					</aside>

					<div class="reviews-body">
						<form class="review-form" onsubmit={addReview}>
							<h3>Leave a review</h3>
							<div class="rf-row">
								<input type="text" placeholder="Your name" bind:value={rvName} aria-label="Your name" required />
								<div class="rf-stars" role="radiogroup" aria-label="Your rating">
									{#each [1, 2, 3, 4, 5] as n (n)}
										<button
											type="button"
											class="rf-star {n <= rvRating ? 'on' : ''}"
											aria-label={n + ' star'}
											onclick={() => (rvRating = n)}>★</button
										>
									{/each}
								</div>
							</div>
							<textarea rows="3" placeholder="How did it arrive? How does it feel?" bind:value={rvText} aria-label="Your review" required></textarea>
							<button class="btn btn-primary" type="submit">Post review</button>
						</form>

						{#if itemReviews.length === 0}
							<p class="no-reviews">No reviews yet. Be the first to share how yours turned out.</p>
						{:else}
							<ul class="review-list">
								{#each itemReviews as r (r.id)}
									<li>
										<div class="rv-top">
											<span class="rv-avatar" style="background:{cw.marigold.yarn}">{r.name.charAt(0)}</span>
											<div>
												<strong>{r.name}</strong>
												<span class="rv-meta">{@render stars(r.rating, 12)} · {r.date}</span>
											</div>
										</div>
										<p>{r.text}</p>
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				</div>
			</section>

			{#if related.length}
				<section class="strip">
					<div class="strip-head"><h2>You might also like</h2></div>
					<div class="grid">
						{#each related as item (item.id)}
							{@render productCard(item)}
						{/each}
					</div>
				</section>
			{/if}
		{/if}

		<!-- ── ADMIN: PATTERN STUDIO ───────────────────────────────────────── -->
		{#if view === 'admin'}
			<section class="page-head admin-head">
				<div>
					<span class="kicker"><Scissors size={14} /> Behind the shop</span>
					<h1>Pattern Studio</h1>
					<p class="lede">Where Jenny writes, tests, and publishes the patterns behind the makes.</p>
				</div>
				<div class="admin-summary">
					<span class="pill st-Published">{statusCounts.Published} published</span>
					<span class="pill st-Testing">{statusCounts.Testing} in testing</span>
					<span class="pill st-Draft">{statusCounts.Draft} draft</span>
				</div>
			</section>

			<div class="admin-bar">
				<form class="search admin-search" onsubmit={(e) => e.preventDefault()}>
					<Search size={16} />
					<input type="search" placeholder="Search patterns" bind:value={adminQuery} aria-label="Search patterns" />
				</form>
				<button class="btn btn-primary" onclick={openNew}><Plus size={17} /> New pattern</button>
			</div>

			<div class="form-wrap" data-open={showForm}>
				<div class="form-inner">
					<form class="pattern-form" onsubmit={savePattern}>
						<div class="pf-head">
							<h3>{editingId ? 'Edit pattern' : 'New pattern'}</h3>
							<button type="button" class="icon-btn" onclick={() => (showForm = false)} aria-label="Close form"><X size={18} /></button>
						</div>
						<div class="pf-grid">
							<label class="full">Pattern name<input type="text" bind:value={fName} placeholder="e.g. Cloud Bunny Amigurumi" required /></label>
							<label>Difficulty
								<select bind:value={fDifficulty}>
									<option>Beginner</option><option>Intermediate</option><option>Advanced</option>
								</select>
							</label>
							<label>Status
								<select bind:value={fStatus}>
									<option>Draft</option><option>Testing</option><option>Published</option>
								</select>
							</label>
							<label>Hook size<input type="text" bind:value={fHook} placeholder="4.0 mm" /></label>
							<label>Yarn weight<input type="text" bind:value={fYarn} placeholder="Worsted" /></label>
							<label>Estimated time<input type="text" bind:value={fTime} placeholder="1 evening" /></label>
							<label class="full">Notes<textarea rows="2" bind:value={fNotes} placeholder="Testing notes, errata, ideas"></textarea></label>
						</div>
						<div class="pf-actions">
							<button type="button" class="btn btn-ghost" onclick={() => (showForm = false)}>Cancel</button>
							<button type="submit" class="btn btn-primary">{editingId ? 'Save changes' : 'Add pattern'}</button>
						</div>
					</form>
				</div>
			</div>

			{#if patternList.length === 0}
				<div class="empty">
					<div class="empty-art">{@render swatch(cw.sage, 'aempty')}</div>
					<h3>No patterns match</h3>
					<p>Clear the search, or add a new pattern.</p>
				</div>
			{:else}
				<div class="table-wrap">
					<table class="patterns">
						<thead>
							<tr><th>Pattern</th><th>Difficulty</th><th>Hook</th><th>Yarn</th><th>Time</th><th>Status</th><th class="ta-r">Actions</th></tr>
						</thead>
						<tbody>
							{#each patternList as p (p.id)}
								<tr>
									<td class="pt-name">{p.name}{#if p.notes}<span class="pt-notes">{p.notes}</span>{/if}</td>
									<td><span class="diff d-{p.difficulty}">{p.difficulty}</span></td>
									<td>{p.hook}</td>
									<td>{p.yarnWeight}</td>
									<td>{p.estTime}</td>
									<td><span class="pill st-{p.status}">{p.status}</span></td>
									<td class="ta-r">
										<button class="row-btn" onclick={() => openEdit(p)} aria-label="Edit pattern"><Pencil size={15} /></button>
										<button class="row-btn danger" onclick={() => deletePattern(p.id)} aria-label="Delete pattern"><Trash2 size={15} /></button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		{/if}
	</main>

	<!-- ── Footer ─────────────────────────────────────────────────────────── -->
	<footer class="site-foot">
		<div class="foot-grid">
			<div class="foot-brand">
				<span class="brand-name">Jenny Gu</span>
				<p>Handmade crochet and a few friends, sent from Plano, Texas, to wherever you are.</p>
				<div class="socials">
					<span class="social"><Instagram size={17} /></span>
					<span class="social"><Mail size={17} /></span>
					<span class="social"><Leaf size={17} /></span>
				</div>
			</div>
			<div class="foot-col">
				<h4>Shop</h4>
				<button onclick={() => go('shop')}>All items</button>
				<button onclick={() => { shopCategory = 'Amigurumi'; go('shop'); }}>Amigurumi</button>
				<button onclick={() => { shopCategory = 'Home'; go('shop'); }}>Home</button>
				<button onclick={() => go('gallery')}>Gallery</button>
			</div>
			<div class="foot-col">
				<h4>Makers</h4>
				<button onclick={() => go('admin')}>Pattern Studio</button>
				<button onclick={() => go('gallery')}>Guest makers</button>
			</div>
			<div class="foot-col">
				<h4>The fine print</h4>
				<span>Made to order, every time</span>
				<span>Tracked worldwide shipping</span>
				<span>Returns within 30 days</span>
			</div>
		</div>
		<div class="foot-base">
			<span>© 2026 Jenny Gu. Every stitch by hand.</span>
			<span class="made">Built with yarn, coffee, and a sleeping cat.</span>
		</div>
	</footer>

	<!-- ── Basket drawer ──────────────────────────────────────────────────── -->
	{#if cartOpen}
		<div class="drawer-overlay" role="presentation" onclick={() => (cartOpen = false)}></div>
		<aside class="drawer" aria-label="Your basket">
			<div class="drawer-head">
				<h3>Your basket</h3>
				<button class="icon-btn" onclick={() => (cartOpen = false)} aria-label="Close basket"><X size={20} /></button>
			</div>
			{#if cartLines.length === 0}
				<div class="drawer-empty">
					<div class="empty-art small">{@render swatch(cw.dustyrose, 'cart')}</div>
					<p>Your basket is empty for now.</p>
					<button class="btn btn-primary" onclick={() => go('shop')}>Start browsing</button>
				</div>
			{:else}
				<ul class="drawer-list">
					{#each cartLines as line (line.item.id)}
						<li>
							<div class="dl-img">{@render swatch(line.item.colorway, 'cl' + line.item.id)}</div>
							<div class="dl-info">
								<strong>{line.item.name}</strong>
								<span>{line.item.maker}</span>
								<div class="dl-qty">
									<button onclick={() => setLineQty(line.item.id, line.qty - 1)} aria-label="Decrease"><Minus size={14} /></button>
									<span>{line.qty}</span>
									<button onclick={() => setLineQty(line.item.id, line.qty + 1)} aria-label="Increase"><Plus size={14} /></button>
								</div>
							</div>
							<div class="dl-right">
								<span class="dl-price">${line.item.price * line.qty}</span>
								<button class="dl-remove" onclick={() => setLineQty(line.item.id, 0)} aria-label="Remove"><Trash2 size={15} /></button>
							</div>
						</li>
					{/each}
				</ul>
				<div class="drawer-foot">
					<div class="dl-total"><span>Subtotal</span><strong>${cartTotal}</strong></div>
					<p class="dl-note">Shipping and any custom options are confirmed at checkout.</p>
					<button class="btn btn-primary big full-w" onclick={() => showToast('This is a demo storefront, checkout is not wired up')}>Checkout</button>
				</div>
			{/if}
		</aside>
	{/if}
</div>

<style>
	.app {
		--paper: oklch(0.97 0.014 78);
		--paper-2: oklch(0.985 0.01 82);
		--card: oklch(0.995 0.006 84);
		--ink: oklch(0.26 0.03 50);
		--ink-soft: oklch(0.44 0.025 55);
		--muted: oklch(0.58 0.02 60);
		--line: oklch(0.88 0.018 72);
		--line-strong: oklch(0.8 0.02 70);
		--marigold: oklch(0.79 0.14 75);
		--rust: oklch(0.56 0.15 52);
		--rust-deep: oklch(0.48 0.15 45);
		--berry: oklch(0.55 0.19 16);
		--berry-soft: oklch(0.93 0.05 18);
		--sage: oklch(0.6 0.06 160);
		--gold: oklch(0.78 0.14 78);

		font-family: 'Hanken Grotesk', ui-sans-serif, system-ui, sans-serif;
		color: var(--ink);
		background:
			radial-gradient(120% 60% at 50% -10%, oklch(0.95 0.04 80) 0%, transparent 60%),
			var(--paper);
		min-height: 100vh;
		line-height: 1.55;
		-webkit-font-smoothing: antialiased;
	}
	.app :global(*) {
		box-sizing: border-box;
	}

	h1, h2, h3, h4 {
		font-family: 'Bricolage Grotesque', 'Hanken Grotesk', sans-serif;
		line-height: 1.06;
		letter-spacing: -0.02em;
		margin: 0;
		font-weight: 800;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}
	button {
		font-family: inherit;
		cursor: pointer;
	}
	.script {
		font-family: 'Caveat', cursive;
		font-weight: 700;
		color: var(--rust);
		letter-spacing: 0;
	}
	.kicker {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.78rem;
		font-weight: 700;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--rust);
		background: oklch(0.94 0.05 70);
		padding: 0.32rem 0.7rem;
		border-radius: 999px;
	}

	/* ── Buttons ── */
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		border: 1px solid transparent;
		border-radius: 0.7rem;
		padding: 0.7rem 1.15rem;
		font-weight: 700;
		font-size: 0.95rem;
		transition: transform 0.4s cubic-bezier(0.22, 1, 0.36, 1), background 0.2s, box-shadow 0.2s;
	}
	.btn:active { transform: translateY(1px); }
	.btn.big { padding: 0.9rem 1.4rem; font-size: 1rem; }
	.btn.full-w { width: 100%; }
	.btn-primary {
		background: var(--rust);
		color: oklch(0.99 0.01 80);
		box-shadow: 0 1px 0 oklch(0.4 0.12 45), 0 10px 22px -12px oklch(0.5 0.14 45 / 0.7);
	}
	.btn-primary:hover { background: var(--rust-deep); transform: translateY(-2px); }
	.btn-ghost {
		background: transparent;
		color: var(--ink);
		border-color: var(--line-strong);
	}
	.btn-ghost:hover { background: var(--paper-2); transform: translateY(-2px); }
	.btn-outline {
		background: var(--card);
		color: var(--ink);
		border-color: var(--line-strong);
	}
	.btn-outline:hover { border-color: var(--rust); color: var(--rust); }

	/* ── Stitch swatch ── */
	.swatch {
		display: block;
		width: 100%;
		height: 100%;
	}

	/* ── Stars ── */
	.stars {
		position: relative;
		display: inline-block;
		line-height: 1;
		white-space: nowrap;
	}
	.s-base { color: oklch(0.85 0.02 75); }
	.s-fill {
		position: absolute;
		inset: 0;
		width: var(--w);
		overflow: hidden;
		color: var(--gold);
	}

	/* ── Topbar ── */
	.topbar {
		position: sticky;
		top: 0;
		z-index: 40;
		display: flex;
		align-items: center;
		gap: 1.2rem;
		padding: 0.7rem clamp(1rem, 4vw, 2.6rem);
		background: oklch(0.97 0.014 78 / 0.86);
		backdrop-filter: blur(10px);
		border-bottom: 1px solid var(--line);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		background: none;
		border: none;
		padding: 0;
	}
	.brand-mark {
		position: relative;
		width: 40px;
		height: 40px;
		border-radius: 12px;
		overflow: hidden;
		border: 2px solid var(--card);
		box-shadow: 0 4px 12px -6px oklch(0.5 0.1 60 / 0.6);
		flex: none;
	}
	.brand-text { display: flex; flex-direction: column; line-height: 1; text-align: left; }
	.brand-name {
		font-family: 'Bricolage Grotesque', sans-serif;
		font-weight: 800;
		font-size: 1.18rem;
		letter-spacing: -0.02em;
	}
	.brand-sub { font-family: 'Caveat', cursive; font-size: 0.95rem; color: var(--rust); }

	.nav { display: flex; gap: 0.2rem; margin-left: 0.5rem; }
	.nav button {
		background: none;
		border: none;
		padding: 0.5rem 0.85rem;
		border-radius: 0.6rem;
		font-weight: 600;
		font-size: 0.95rem;
		color: var(--ink-soft);
		position: relative;
	}
	.nav button:hover { color: var(--ink); background: var(--paper-2); }
	.nav button.active { color: var(--rust); }
	.nav button.active::after {
		content: '';
		position: absolute;
		left: 0.85rem;
		right: 0.85rem;
		bottom: 0.2rem;
		height: 2px;
		border-radius: 2px;
		background: var(--rust);
	}

	.tools { display: flex; align-items: center; gap: 0.5rem; margin-left: auto; }
	.search {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		background: var(--card);
		border: 1px solid var(--line);
		border-radius: 0.7rem;
		padding: 0.45rem 0.7rem;
		color: var(--muted);
	}
	.search input {
		border: none;
		background: none;
		outline: none;
		font-size: 0.9rem;
		color: var(--ink);
		width: 11rem;
	}
	.search:focus-within { border-color: var(--rust); }
	.icon-btn {
		position: relative;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		border-radius: 0.65rem;
		border: 1px solid transparent;
		background: none;
		color: var(--ink);
	}
	.icon-btn:hover { background: var(--paper-2); }
	.count {
		position: absolute;
		top: 1px;
		right: 1px;
		min-width: 17px;
		height: 17px;
		padding: 0 4px;
		border-radius: 999px;
		background: var(--sage);
		color: white;
		font-size: 0.68rem;
		font-weight: 700;
		display: grid;
		place-items: center;
	}
	.count.accent { background: var(--berry); }

	/* ── Toast ── */
	.toast {
		position: fixed;
		bottom: 1.5rem;
		left: 50%;
		transform: translateX(-50%);
		z-index: 80;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: var(--ink);
		color: oklch(0.97 0.02 80);
		padding: 0.7rem 1.1rem;
		border-radius: 0.8rem;
		font-weight: 600;
		font-size: 0.9rem;
		box-shadow: 0 18px 40px -16px oklch(0.3 0.05 50 / 0.8);
		animation: pop 0.5s cubic-bezier(0.22, 1, 0.36, 1);
	}
	.toast :global(svg) { color: var(--marigold); }

	main { width: min(1180px, 92%); margin: 0 auto; padding: clamp(2rem, 5vw, 4rem) 0 4rem; }

	/* ── Hero ── */
	.hero {
		display: grid;
		grid-template-columns: 1.05fr 0.95fr;
		gap: clamp(1.5rem, 4vw, 3.5rem);
		align-items: center;
		padding-bottom: clamp(2rem, 5vw, 4rem);
	}
	.hero-copy { animation: rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) both; }
	.hero h1 {
		font-size: clamp(2.5rem, 6vw, 4.3rem);
		margin: 1rem 0 1.1rem;
		max-width: 16ch;
	}
	.hero-copy p { font-size: 1.08rem; color: var(--ink-soft); max-width: 46ch; }
	.hero-actions { display: flex; gap: 0.8rem; margin: 1.6rem 0 1.4rem; flex-wrap: wrap; }
	.hero-trust { display: flex; flex-wrap: wrap; gap: 1.1rem; color: var(--muted); font-size: 0.88rem; font-weight: 600; }
	.hero-trust span { display: inline-flex; align-items: center; gap: 0.35rem; }
	.hero-trust :global(svg) { color: var(--rust); }

	.hero-art {
		position: relative;
		display: grid;
		grid-template-columns: 1fr 1fr;
		grid-auto-rows: 130px;
		gap: 0.8rem;
		animation: rise 0.9s cubic-bezier(0.22, 1, 0.36, 1) both;
	}
	.hero-tile {
		border-radius: 1.1rem;
		overflow: hidden;
		box-shadow: 0 18px 40px -22px oklch(0.45 0.1 55 / 0.6);
		transform: rotate(-1deg);
	}
	.hero-tile:nth-child(2) { transform: rotate(2deg); }
	.hero-tile:nth-child(3) { transform: rotate(-2deg); }
	.hero-tile.big { grid-row: span 2; transform: rotate(1.5deg); }
	.hero-tile.wide { grid-column: span 2; grid-row: span 1; }
	.hero-sticker {
		position: absolute;
		bottom: -0.8rem;
		right: -0.6rem;
		background: var(--berry);
		color: white;
		font-family: 'Caveat', cursive;
		font-size: 1.25rem;
		padding: 0.4rem 1rem;
		border-radius: 999px;
		transform: rotate(-5deg);
		box-shadow: 0 10px 24px -10px oklch(0.4 0.15 18 / 0.8);
	}

	/* ── Section shells ── */
	.strip { padding: clamp(1.8rem, 4vw, 3rem) 0; }
	.strip-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1.3rem; }
	.strip-head h2, .categories h2, .how h2 { font-size: clamp(1.5rem, 3.2vw, 2.1rem); }
	.link, .inline-link {
		background: none;
		border: none;
		color: var(--rust);
		font-weight: 700;
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0;
	}
	.inline-link { text-decoration: underline; text-underline-offset: 3px; }
	.link:hover, .inline-link:hover { color: var(--rust-deep); }

	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(230px, 1fr)); gap: 1.2rem; }
	.wide-grid { grid-template-columns: repeat(auto-fill, minmax(245px, 1fr)); }

	/* ── Product card ── */
	.card {
		background: var(--card);
		border: 1px solid var(--line);
		border-radius: 1.1rem;
		overflow: hidden;
		text-align: left;
		transition: transform 0.45s cubic-bezier(0.22, 1, 0.36, 1), box-shadow 0.3s, border-color 0.3s;
	}
	.card:hover {
		transform: translateY(-5px);
		box-shadow: 0 22px 44px -26px oklch(0.45 0.08 55 / 0.7);
		border-color: var(--line-strong);
	}
	.card:focus-visible { outline: 2.5px solid var(--rust); outline-offset: 2px; }
	.card-media { position: relative; aspect-ratio: 5 / 4; overflow: hidden; }
	.card-media .swatch { transition: transform 0.6s cubic-bezier(0.22, 1, 0.36, 1); }
	.card:hover .card-media .swatch { transform: scale(1.05); }
	.ribbon {
		position: absolute;
		top: 0.7rem;
		left: 0.7rem;
		background: var(--ink);
		color: oklch(0.97 0.02 80);
		font-size: 0.72rem;
		font-weight: 700;
		letter-spacing: 0.02em;
		padding: 0.3rem 0.6rem;
		border-radius: 0.5rem;
	}
	.fav {
		position: absolute;
		top: 0.6rem;
		right: 0.6rem;
		width: 36px;
		height: 36px;
		border-radius: 999px;
		border: none;
		background: oklch(0.99 0.01 80 / 0.92);
		color: var(--ink-soft);
		display: grid;
		place-items: center;
		box-shadow: 0 4px 12px -6px oklch(0.4 0.05 50 / 0.6);
		transition: transform 0.3s, color 0.2s;
	}
	.fav:hover { transform: scale(1.1); color: var(--berry); }
	.fav.on { color: var(--berry); }
	.fav.big { width: 46px; height: 46px; top: 1rem; right: 1rem; }
	.card-body { padding: 0.85rem 0.95rem 1rem; }
	.card-maker {
		font-size: 0.78rem;
		color: var(--muted);
		font-weight: 600;
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin-bottom: 0.25rem;
	}
	.guest-tag {
		font-size: 0.66rem;
		background: oklch(0.92 0.05 300);
		color: oklch(0.45 0.13 305);
		padding: 0.1rem 0.4rem;
		border-radius: 0.4rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.card-body h3 { font-size: 1.02rem; font-weight: 700; font-family: 'Hanken Grotesk', sans-serif; letter-spacing: -0.01em; }
	.card-foot { display: flex; align-items: center; justify-content: space-between; margin-top: 0.6rem; }
	.price { font-weight: 800; font-size: 1.1rem; font-family: 'Bricolage Grotesque', sans-serif; }
	.rate { display: inline-flex; align-items: center; gap: 0.35rem; }
	.rate em { font-style: normal; font-size: 0.8rem; color: var(--muted); }

	/* ── Categories ── */
	.categories { padding: clamp(1.5rem, 4vw, 2.6rem) 0; }
	.categories h2 { margin-bottom: 1.3rem; }
	.cat-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 1rem; }
	.cat { border: none; background: none; padding: 0; display: block; }
	.cat-art {
		aspect-ratio: 4 / 3;
		border-radius: 1rem;
		overflow: hidden;
		border: 1px solid var(--line);
		transition: transform 0.45s cubic-bezier(0.22, 1, 0.36, 1);
	}
	.cat:hover .cat-art { transform: translateY(-4px) rotate(-1deg); }
	.cat span { display: block; margin-top: 0.6rem; font-weight: 700; }

	/* ── About ── */
	.about {
		display: grid;
		grid-template-columns: 0.9fr 1.1fr;
		gap: clamp(1.5rem, 4vw, 3rem);
		align-items: center;
		padding: clamp(2rem, 5vw, 3.5rem) 0;
		margin: 1.5rem 0;
		border-top: 1px solid var(--line);
		border-bottom: 1px solid var(--line);
	}
	.about-photo {
		position: relative;
		aspect-ratio: 4 / 5;
		border-radius: 1.4rem;
		overflow: hidden;
		box-shadow: 0 30px 60px -34px oklch(0.4 0.08 55 / 0.7);
	}
	.about-fallback { position: absolute; inset: 0; }
	.about-photo img {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		transition: opacity 0.4s;
	}
	.about-copy h2 { font-size: clamp(1.6rem, 3.4vw, 2.4rem); margin: 0.9rem 0 1rem; max-width: 18ch; }
	.about-copy p { color: var(--ink-soft); max-width: 50ch; margin: 0 0 0.9rem; }
	.about-stats { display: flex; gap: 2rem; margin-top: 1.4rem; }
	.about-stats div { display: flex; flex-direction: column; }
	.about-stats strong { font-family: 'Bricolage Grotesque', sans-serif; font-size: 1.5rem; }
	.about-stats span { font-size: 0.82rem; color: var(--muted); }

	/* ── How ── */
	.how { padding: clamp(1.5rem, 4vw, 3rem) 0; }
	.how h2 { margin-bottom: 1.6rem; }
	.steps { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 1.4rem; counter-reset: s; }
	.steps li { position: relative; padding-top: 1rem; }
	.step-n {
		font-family: 'Bricolage Grotesque', sans-serif;
		font-size: 2.4rem;
		font-weight: 800;
		color: var(--marigold);
		line-height: 1;
	}
	.steps h3 { font-size: 1.15rem; margin: 0.5rem 0 0.4rem; font-family: 'Hanken Grotesk', sans-serif; font-weight: 700; }
	.steps p { color: var(--ink-soft); font-size: 0.95rem; max-width: 32ch; }

	/* ── Newsletter ── */
	.news {
		background: var(--ink);
		border-radius: 1.6rem;
		padding: clamp(1.6rem, 4vw, 2.8rem);
		margin: 1.5rem 0;
		color: oklch(0.95 0.02 80);
		overflow: hidden;
		position: relative;
	}
	.news::before {
		content: '';
		position: absolute;
		inset: 0;
		background: radial-gradient(80% 120% at 100% 0%, oklch(0.6 0.13 55 / 0.45), transparent 55%);
	}
	.news-inner { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 2rem; flex-wrap: wrap; }
	.news h2 { color: oklch(0.97 0.02 80); font-size: clamp(1.4rem, 3vw, 2rem); }
	.news p { color: oklch(0.84 0.02 80); margin-top: 0.4rem; max-width: 40ch; }
	.news-form {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		background: oklch(0.99 0.01 80);
		border-radius: 0.8rem;
		padding: 0.4rem 0.4rem 0.4rem 0.8rem;
		color: var(--muted);
	}
	.news-form input { border: none; background: none; outline: none; padding: 0.5rem; color: var(--ink); width: 14rem; max-width: 50vw; }

	/* ── Page heads ── */
	.page-head { padding-bottom: 1.4rem; margin-bottom: 0.6rem; }
	.page-head h1 { font-size: clamp(2rem, 4.5vw, 3.1rem); margin: 0.8rem 0 0.6rem; }
	.lede { color: var(--ink-soft); font-size: 1.05rem; max-width: 56ch; }
	.gallery-head, .admin-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 1.5rem; flex-wrap: wrap; }

	/* ── Controls ── */
	.controls { display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-bottom: 1.6rem; flex-wrap: wrap; }
	.chips { display: flex; gap: 0.5rem; flex-wrap: wrap; }
	.chip {
		border: 1px solid var(--line-strong);
		background: var(--card);
		color: var(--ink-soft);
		padding: 0.45rem 0.9rem;
		border-radius: 999px;
		font-weight: 600;
		font-size: 0.9rem;
		transition: all 0.2s;
	}
	.chip:hover { border-color: var(--rust); color: var(--rust); }
	.chip.on { background: var(--ink); color: oklch(0.97 0.02 80); border-color: var(--ink); }
	.sort { display: inline-flex; align-items: center; gap: 0.5rem; font-size: 0.9rem; color: var(--muted); font-weight: 600; }
	.sort select, .pf-grid select {
		font-family: inherit;
		border: 1px solid var(--line-strong);
		background: var(--card);
		color: var(--ink);
		border-radius: 0.6rem;
		padding: 0.5rem 0.7rem;
		font-weight: 600;
	}

	/* ── Empty ── */
	.empty { text-align: center; padding: 3rem 1rem; }
	.empty-art { width: 120px; height: 96px; border-radius: 1rem; overflow: hidden; margin: 0 auto 1rem; opacity: 0.8; }
	.empty-art.small { width: 96px; height: 76px; }
	.empty h3 { font-size: 1.3rem; }
	.empty p { color: var(--muted); margin: 0.4rem 0 1.2rem; }

	/* ── Gallery masonry ── */
	.masonry { columns: 4 230px; column-gap: 1.1rem; }
	.tile {
		break-inside: avoid;
		margin: 0 0 1.1rem;
		border-radius: 1.1rem;
		overflow: hidden;
		background: var(--card);
		border: 1px solid var(--line);
		position: relative;
		transition: transform 0.45s cubic-bezier(0.22, 1, 0.36, 1), box-shadow 0.3s;
	}
	.tile:hover { transform: translateY(-4px); box-shadow: 0 20px 40px -24px oklch(0.45 0.08 55 / 0.7); }
	.tile:focus-visible { outline: 2.5px solid var(--rust); outline-offset: 2px; }
	.tile-media { position: relative; aspect-ratio: 1 / 1; }
	.tile.tall .tile-media { aspect-ratio: 3 / 4; }
	.tile.short .tile-media { aspect-ratio: 4 / 3; }
	.tile-cap { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; padding: 0.7rem 0.85rem 0.85rem; }
	.tile-cap strong { display: block; font-size: 0.92rem; }
	.tile-maker { font-size: 0.76rem; color: var(--muted); }
	.tile-price { font-weight: 800; font-family: 'Bricolage Grotesque', sans-serif; }
	.guest-corner {
		position: absolute;
		top: 0.6rem;
		left: 0.6rem;
		background: oklch(0.99 0.01 80 / 0.92);
		color: oklch(0.45 0.13 305);
		font-size: 0.66rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 0.2rem 0.5rem;
		border-radius: 0.4rem;
	}
	.seg, .admin-summary { display: flex; gap: 0.35rem; }
	.seg { background: var(--paper-2); border: 1px solid var(--line); border-radius: 0.8rem; padding: 0.25rem; }
	.seg button {
		border: none;
		background: none;
		padding: 0.45rem 0.85rem;
		border-radius: 0.6rem;
		font-weight: 600;
		font-size: 0.88rem;
		color: var(--ink-soft);
	}
	.seg button.on { background: var(--card); color: var(--ink); box-shadow: 0 2px 8px -4px oklch(0.4 0.05 50 / 0.5); }

	/* ── Product detail ── */
	.crumbs { display: flex; align-items: center; gap: 0.5rem; font-size: 0.85rem; color: var(--muted); margin-bottom: 1.3rem; }
	.crumbs button { background: none; border: none; color: var(--muted); padding: 0; }
	.crumbs button:hover { color: var(--rust); }
	.crumbs em { font-style: normal; color: var(--ink); font-weight: 600; }
	.product { display: grid; grid-template-columns: 1.05fr 1fr; gap: clamp(1.5rem, 4vw, 3rem); align-items: start; }
	.product-media { position: sticky; top: 5rem; }
	.main-img { position: relative; aspect-ratio: 1 / 1; border-radius: 1.4rem; overflow: hidden; box-shadow: 0 30px 60px -34px oklch(0.4 0.08 55 / 0.6); }
	.thumbs { display: grid; grid-template-columns: repeat(4, 1fr); gap: 0.6rem; margin-top: 0.7rem; }
	.thumb { aspect-ratio: 1 / 1; border-radius: 0.7rem; overflow: hidden; border: 2px solid transparent; padding: 0; background: none; transition: border-color 0.2s; }
	.thumb.on { border-color: var(--rust); }

	.maker-line { display: flex; align-items: center; gap: 0.6rem; font-weight: 600; color: var(--ink-soft); }
	.star-seller { display: inline-flex; align-items: center; gap: 0.3rem; font-size: 0.78rem; color: var(--rust); background: oklch(0.94 0.05 70); padding: 0.2rem 0.5rem; border-radius: 999px; }
	.product-info h1 { font-size: clamp(1.8rem, 3.6vw, 2.7rem); margin: 0.6rem 0; }
	.prod-rate { display: flex; align-items: center; gap: 0.6rem; margin-bottom: 1rem; }
	.prod-rate strong { font-size: 1rem; }
	.price-line { display: flex; align-items: baseline; gap: 0.8rem; margin-bottom: 1.1rem; }
	.big-price { font-family: 'Bricolage Grotesque', sans-serif; font-size: 2.2rem; font-weight: 800; }
	.vat { color: var(--muted); font-size: 0.9rem; }
	.prod-desc { color: var(--ink-soft); font-size: 1.02rem; margin-bottom: 1.5rem; max-width: 54ch; }

	.buy { display: flex; align-items: center; gap: 0.7rem; flex-wrap: wrap; margin-bottom: 1.6rem; }
	.qty { display: inline-flex; align-items: center; border: 1px solid var(--line-strong); border-radius: 0.7rem; overflow: hidden; }
	.qty button { width: 42px; height: 46px; border: none; background: var(--card); color: var(--ink); display: grid; place-items: center; }
	.qty button:hover { background: var(--paper-2); }
	.qty span { width: 40px; text-align: center; font-weight: 700; }

	.specs { display: grid; gap: 0.1rem; border: 1px solid var(--line); border-radius: 1rem; overflow: hidden; margin-bottom: 1.5rem; }
	.specs div { display: grid; grid-template-columns: 11rem 1fr; gap: 1rem; padding: 0.85rem 1.1rem; background: var(--card); }
	.specs div:nth-child(even) { background: var(--paper-2); }
	.specs dt { display: inline-flex; align-items: center; gap: 0.45rem; font-weight: 700; color: var(--ink); margin: 0; }
	.specs dt :global(svg) { color: var(--rust); }
	.specs dd { margin: 0; color: var(--ink-soft); }

	.meet-maker { display: flex; gap: 0.9rem; align-items: center; background: var(--paper-2); border: 1px solid var(--line); border-radius: 1rem; padding: 1rem; }
	.mm-avatar { width: 56px; height: 56px; border-radius: 999px; overflow: hidden; flex: none; border: 2px solid var(--card); }
	.meet-maker strong { font-family: 'Bricolage Grotesque', sans-serif; }
	.meet-maker p { margin: 0.2rem 0 0; font-size: 0.9rem; color: var(--ink-soft); }

	/* ── Reviews ── */
	.reviews { margin-top: clamp(2.5rem, 6vw, 4rem); padding-top: 2rem; border-top: 1px solid var(--line); }
	.reviews-head h2 { font-size: clamp(1.5rem, 3.2vw, 2.1rem); margin-bottom: 1.4rem; }
	.reviews-layout { display: grid; grid-template-columns: 260px 1fr; gap: clamp(1.5rem, 4vw, 3rem); align-items: start; }
	.rate-summary { position: sticky; top: 5rem; }
	.big-rating { text-align: center; padding: 1.3rem; background: var(--card); border: 1px solid var(--line); border-radius: 1rem; margin-bottom: 1rem; }
	.big-rating strong { display: block; font-family: 'Bricolage Grotesque', sans-serif; font-size: 3rem; line-height: 1; }
	.big-rating span { display: block; margin-top: 0.4rem; color: var(--muted); font-size: 0.85rem; }
	.bars { display: grid; gap: 0.4rem; }
	.bar-row { display: grid; grid-template-columns: 1.8rem 1fr 1.5rem; align-items: center; gap: 0.5rem; font-size: 0.82rem; color: var(--muted); }
	.bar { height: 7px; background: var(--line); border-radius: 999px; overflow: hidden; }
	.bar i { display: block; height: 100%; background: var(--gold); border-radius: 999px; }

	.review-form { background: var(--paper-2); border: 1px solid var(--line); border-radius: 1rem; padding: 1.2rem; margin-bottom: 1.6rem; }
	.review-form h3 { font-size: 1.1rem; margin-bottom: 0.8rem; font-family: 'Hanken Grotesk', sans-serif; font-weight: 700; }
	.rf-row { display: flex; gap: 0.8rem; align-items: center; margin-bottom: 0.7rem; flex-wrap: wrap; }
	.rf-row input, .review-form textarea {
		font-family: inherit;
		border: 1px solid var(--line-strong);
		background: var(--card);
		border-radius: 0.6rem;
		padding: 0.6rem 0.75rem;
		color: var(--ink);
		font-size: 0.95rem;
		outline: none;
	}
	.rf-row input { flex: 1; min-width: 12rem; }
	.review-form textarea { width: 100%; resize: vertical; margin-bottom: 0.8rem; }
	.rf-row input:focus, .review-form textarea:focus { border-color: var(--rust); }
	.rf-stars { display: flex; gap: 0.1rem; }
	.rf-star { background: none; border: none; font-size: 1.5rem; color: oklch(0.85 0.02 75); padding: 0 0.05rem; line-height: 1; }
	.rf-star.on { color: var(--gold); }

	.no-reviews { color: var(--muted); padding: 1rem 0; }
	.review-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 1.2rem; }
	.review-list li { border-bottom: 1px solid var(--line); padding-bottom: 1.2rem; }
	.rv-top { display: flex; align-items: center; gap: 0.7rem; margin-bottom: 0.5rem; }
	.rv-avatar { width: 38px; height: 38px; border-radius: 999px; display: grid; place-items: center; font-weight: 800; color: oklch(0.3 0.05 60); font-family: 'Bricolage Grotesque', sans-serif; }
	.rv-meta { display: flex; align-items: center; gap: 0.4rem; font-size: 0.8rem; color: var(--muted); }
	.review-list p { margin: 0; color: var(--ink-soft); }

	/* ── Admin ── */
	.admin-summary .pill { font-size: 0.82rem; }
	.pill { display: inline-flex; align-items: center; padding: 0.3rem 0.7rem; border-radius: 999px; font-weight: 700; font-size: 0.78rem; }
	.st-Published { background: oklch(0.92 0.06 150); color: oklch(0.42 0.09 158); }
	.st-Testing { background: oklch(0.93 0.07 80); color: oklch(0.45 0.12 70); }
	.st-Draft { background: oklch(0.9 0.015 60); color: oklch(0.45 0.02 60); }

	.admin-bar { display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-bottom: 1.2rem; flex-wrap: wrap; }
	.admin-search input { width: 16rem; max-width: 60vw; }

	.form-wrap { display: grid; grid-template-rows: 0fr; transition: grid-template-rows 0.45s cubic-bezier(0.22, 1, 0.36, 1); }
	.form-wrap[data-open='true'] { grid-template-rows: 1fr; margin-bottom: 1.4rem; }
	.form-inner { overflow: hidden; }
	.pattern-form { background: var(--card); border: 1px solid var(--line-strong); border-radius: 1.1rem; padding: 1.3rem; }
	.pf-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1rem; }
	.pf-head h3 { font-size: 1.2rem; }
	.pf-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.9rem; }
	.pf-grid label { display: flex; flex-direction: column; gap: 0.35rem; font-size: 0.85rem; font-weight: 700; color: var(--ink-soft); }
	.pf-grid label.full { grid-column: 1 / -1; }
	.pf-grid input, .pf-grid textarea, .pf-grid select {
		font-family: inherit;
		border: 1px solid var(--line-strong);
		background: var(--paper-2);
		border-radius: 0.6rem;
		padding: 0.55rem 0.7rem;
		color: var(--ink);
		font-size: 0.95rem;
		font-weight: 500;
		outline: none;
	}
	.pf-grid input:focus, .pf-grid textarea:focus, .pf-grid select:focus { border-color: var(--rust); background: var(--card); }
	.pf-actions { display: flex; justify-content: flex-end; gap: 0.6rem; margin-top: 1rem; }

	.table-wrap { border: 1px solid var(--line); border-radius: 1.1rem; overflow: hidden; background: var(--card); }
	.patterns { width: 100%; border-collapse: collapse; }
	.patterns thead th { text-align: left; font-size: 0.74rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); padding: 0.85rem 1rem; background: var(--paper-2); border-bottom: 1px solid var(--line); }
	.patterns td { padding: 0.9rem 1rem; border-bottom: 1px solid var(--line); font-size: 0.92rem; vertical-align: top; }
	.patterns tbody tr:last-child td { border-bottom: none; }
	.patterns tbody tr:hover { background: var(--paper-2); }
	.ta-r { text-align: right; }
	.pt-name { font-weight: 700; max-width: 22rem; }
	.pt-notes { display: block; font-weight: 400; font-size: 0.8rem; color: var(--muted); margin-top: 0.2rem; }
	.diff { font-weight: 700; font-size: 0.82rem; }
	.d-Beginner { color: var(--sage); }
	.d-Intermediate { color: var(--rust); }
	.d-Advanced { color: var(--berry); }
	.row-btn { width: 34px; height: 34px; border-radius: 0.55rem; border: 1px solid var(--line); background: var(--card); color: var(--ink-soft); display: inline-grid; place-items: center; margin-left: 0.3rem; transition: all 0.2s; }
	.row-btn:hover { border-color: var(--rust); color: var(--rust); }
	.row-btn.danger:hover { border-color: var(--berry); color: var(--berry); background: var(--berry-soft); }

	/* ── Footer ── */
	.site-foot { border-top: 1px solid var(--line); background: var(--paper-2); margin-top: 3rem; }
	.foot-grid { width: min(1180px, 92%); margin: 0 auto; padding: 3rem 0 2rem; display: grid; grid-template-columns: 1.6fr 1fr 1fr 1fr; gap: 2rem; }
	.foot-brand .brand-name { font-size: 1.3rem; font-family: 'Bricolage Grotesque', sans-serif; }
	.foot-brand p { color: var(--ink-soft); font-size: 0.92rem; max-width: 34ch; margin: 0.6rem 0 1rem; }
	.socials { display: flex; gap: 0.5rem; }
	.social { width: 38px; height: 38px; border-radius: 999px; display: grid; place-items: center; background: var(--card); border: 1px solid var(--line); color: var(--ink-soft); }
	.foot-col { display: flex; flex-direction: column; gap: 0.55rem; }
	.foot-col h4 { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin-bottom: 0.3rem; font-family: 'Hanken Grotesk', sans-serif; }
	.foot-col button, .foot-col span { background: none; border: none; text-align: left; padding: 0; color: var(--ink-soft); font-size: 0.92rem; font-family: inherit; }
	.foot-col button:hover { color: var(--rust); }
	.foot-base { border-top: 1px solid var(--line); }
	.foot-base { width: min(1180px, 92%); margin: 0 auto; padding: 1.2rem 0; display: flex; justify-content: space-between; gap: 1rem; flex-wrap: wrap; font-size: 0.84rem; color: var(--muted); }
	.foot-base .made { font-family: 'Caveat', cursive; font-size: 1.05rem; color: var(--rust); }

	/* ── Drawer ── */
	.drawer-overlay { position: fixed; inset: 0; background: oklch(0.3 0.03 50 / 0.4); z-index: 60; animation: fade 0.3s ease; }
	.drawer {
		position: fixed;
		top: 0;
		right: 0;
		bottom: 0;
		width: min(420px, 92vw);
		background: var(--paper);
		z-index: 70;
		display: flex;
		flex-direction: column;
		box-shadow: -20px 0 50px -20px oklch(0.3 0.05 50 / 0.5);
		animation: slide 0.45s cubic-bezier(0.22, 1, 0.36, 1);
	}
	.drawer-head { display: flex; align-items: center; justify-content: space-between; padding: 1.2rem 1.3rem; border-bottom: 1px solid var(--line); }
	.drawer-head h3 { font-size: 1.25rem; }
	.drawer-empty { text-align: center; padding: 3rem 1.5rem; display: flex; flex-direction: column; align-items: center; gap: 0.8rem; }
	.drawer-list { list-style: none; margin: 0; padding: 0.5rem; overflow-y: auto; flex: 1; }
	.drawer-list li { display: flex; gap: 0.8rem; padding: 0.8rem; border-radius: 0.9rem; }
	.drawer-list li:hover { background: var(--paper-2); }
	.dl-img { width: 70px; height: 70px; border-radius: 0.7rem; overflow: hidden; flex: none; }
	.dl-info { flex: 1; display: flex; flex-direction: column; gap: 0.2rem; }
	.dl-info strong { font-size: 0.95rem; }
	.dl-info span { font-size: 0.8rem; color: var(--muted); }
	.dl-qty { display: inline-flex; align-items: center; gap: 0.3rem; margin-top: 0.3rem; }
	.dl-qty button { width: 26px; height: 26px; border-radius: 0.45rem; border: 1px solid var(--line-strong); background: var(--card); display: grid; place-items: center; color: var(--ink); }
	.dl-qty span { width: 26px; text-align: center; font-weight: 700; font-size: 0.9rem; }
	.dl-right { display: flex; flex-direction: column; align-items: flex-end; justify-content: space-between; }
	.dl-price { font-weight: 800; font-family: 'Bricolage Grotesque', sans-serif; }
	.dl-remove { background: none; border: none; color: var(--muted); padding: 0.2rem; }
	.dl-remove:hover { color: var(--berry); }
	.drawer-foot { padding: 1.2rem 1.3rem; border-top: 1px solid var(--line); background: var(--card); }
	.dl-total { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 0.4rem; }
	.dl-total strong { font-family: 'Bricolage Grotesque', sans-serif; font-size: 1.5rem; }
	.dl-note { font-size: 0.8rem; color: var(--muted); margin: 0 0 0.9rem; }

	/* ── Animations ── */
	@keyframes rise { from { opacity: 0; transform: translateY(22px); } to { opacity: 1; transform: translateY(0); } }
	@keyframes pop { from { opacity: 0; transform: translate(-50%, 12px); } to { opacity: 1; transform: translate(-50%, 0); } }
	@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
	@keyframes slide { from { transform: translateX(100%); } to { transform: translateX(0); } }

	/* ── Responsive ── */
	@media (max-width: 960px) {
		.hero, .about, .product, .reviews-layout { grid-template-columns: 1fr; }
		.product-media, .rate-summary { position: static; }
		.nav { display: none; }
		.search input { width: 7rem; }
		.masonry { columns: 2 160px; }
		.pf-grid { grid-template-columns: 1fr 1fr; }
		.foot-grid { grid-template-columns: 1fr 1fr; }
	}
	@media (max-width: 560px) {
		.brand-sub { display: none; }
		.hero h1 { font-size: 2.3rem; }
		.specs div { grid-template-columns: 1fr; gap: 0.2rem; }
		.pf-grid { grid-template-columns: 1fr; }
		.foot-grid { grid-template-columns: 1fr; }
		.btn.big { flex: 1; }
		.controls { flex-direction: column; align-items: stretch; }
	}
</style>
