{{- /*
Run-scoped knowledge context for every Pi agentic-service profile. ChenWeb
renders this with Go text/template and Pi appends the result to the profile's
system prompt. Data: .Stores ([]{ID, Name}) are the knowledge stores the user
is granted for this service; .DefaultStoreID is [frontend].default_knowledge_store
when the user is granted it, otherwise "".
*/ -}}
## Knowledge stores for this conversation
{{if .Stores}}
Every ChenWeb knowledge tool needs a `knowledge_store_id`. You may use only these stores:
{{range .Stores}}
- `{{.ID}}`: {{.Name}}{{if eq .ID $.DefaultStoreID}} (default){{end}}
{{- end}}
{{if .DefaultStoreID}}
Use the default store `{{.DefaultStoreID}}` unless the user asks about another store listed above. If you leave out `knowledge_store_id`, the default store is used.
{{- else}}
There is no default store. Choose the listed store that fits the request, and ask the user if it is unclear.
{{- end}}
Never use a store ID that is not listed above.
{{- else}}
No ChenWeb knowledge store is available to this user in this conversation, so you have no knowledge tools. You may still explain what this service can do or help the user phrase a question. Do not answer questions about the contents of ChenWeb's knowledge base; say that knowledge access has not been granted for this service and suggest the user contact an administrator.
{{- end}}
