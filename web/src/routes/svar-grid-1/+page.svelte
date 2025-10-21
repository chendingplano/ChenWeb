<script lang="ts">
	import { Grid } from "@svar-ui/svelte-grid";

	// let gridEl = $state<HTMLDivElement | null>(null)
	const data_01 = [
		{
			id: 1,
			email: "Leora13@yahoo.com",
			firstName: "Ernest",
			lastName: "Schuppe",
		},
		{
			id: 2,
			email: "Mose_Gerhold51@yahoo.com",
			firstName: "Janis",
			lastName: "Vandervort",
		},
		{
			id: 3,
			email: "Mose_abc@yahoo.com",
			firstName: "Michael",
			lastName: "Smith",
		},
	];

	const data = [
		{
			id: 1,
			firstName: "Chen",
			lastName: "Ding",
			email: "chending0602@hotmail.com",
			companyName: "Zykie Networks",
			stars: "5",
			date: "20250101"
		}
	]

	const columns_01 = [
        {
            id: "id",
            width: 50
        },
        {
            id: "firstName",
            header: "First Name",
            footer: "First Name",
            width: 150,
        },
        {
            id: "lastName",
            header: "Last Name",
            footer: "Last Name",
            width: 150,
        },
    ];

	const columns02 = [
       { id: "id", width: 50 },
       { id: "firstName", header: "First Name", footer: "First Name", width: 150 },
       { id: "email", header: "Email", footer: "Email" },
       { id: "companyName", header: "Company", footer: "Company" },
       { id: "stars", header: "Stars", width: 150, hidden: true },
       { id: "date", header: "Date", width: 150 },
    ];

    const columns = [
        { id: "id", width: 50, footer: { text: "All users", colspan: 4 } },
        {
            id: "firstName",
            header: [
                { text: "Main client info", colspan: 3 },
                { text: "User", colspan: 2 },
                { text: "First Name" },
            ],
            width: 150,
        },
        {
            id: "lastName",
            header: ["", "", "Last Name"],
            width: 150,
        },
        {
            id: "companyName",
            header: [
                "",
                { text: "Company" },
                { text: "Name" },
            ],
        },
    ];

</script>

<div style="padding: 200px; width: 1500px; height: 1000px">
	<div>
		<Grid {data} {columns}/>
	</div>
</div>
