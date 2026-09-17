" Vim syntax file for the zen schema DSL (.zen files).
" Language: zen (github.com/zenta-dev/zever internal/dsl)
"
" NOTE FOR MAINTAINERS: the keyword/type/label word lists below must be kept
" in sync with the sibling TextMate grammar at
" editors/vscode/syntaxes/zen.tmLanguage.json (built in a parallel PR). If
" you add/remove a reserved word, builtin type, or contextual block label
" here, mirror the change there too.

if exists("b:current_syntax")
  finish
endif

" Reserved keywords -- see internal/dsl/token/token.go's Keywords map. This
" is the authoritative, lexer-level reserved word list (14 words: the 12
" structural keywords plus true/false, handled separately below).
" "module" is intentionally absent: module-block syntax was removed in
" v-next (modules are now derived from the directory), so it lexes as a
" plain identifier.
syntax keyword zenKeyword entity service job schedule rpc message index enum has_many has_one belongs_to many_to_many

syntax keyword zenBoolean true false

" Builtin scalar types (plain identifiers at the lexer level, not
" reserved -- see internal/dsl/resolver/resolver_entity.go's type table).
syntax match zenType "\<\%(uuid\|string\|int32\|int64\|float32\|float64\|bool\|timestamp\|date\|bytes\|json\)\>"

" Contextual block labels used inside rpc/job/schedule bodies (http, auth,
" permission, queue, retry, cron, dispatch, join_table -- see
" internal/dsl/parser/parser_service.go, parser_job.go, parser_schedule.go,
" parser_entity.go) plus the attribute names the resolver understands
" (foreign_key, on_delete, validate, default, primary, unique, required --
" see resolver_entity.go, resolver_relation.go, resolver_service.go,
" resolver_message.go). These are plain identifiers to the lexer, only
" meaningful by position, but it's helpful to color them distinctly.
syntax match zenLabel "\<\%(http\|auth\|permission\|queue\|retry\|cron\|dispatch\|join_table\|foreign_key\|on_delete\|validate\|default\|primary\|unique\|required\)\>\s*:"me=e-1

" HTTP verbs following an `http:` label, e.g. `http: GET "/tasks"`.
syntax keyword zenHTTPVerb GET POST PUT PATCH DELETE

syntax match zenComment "//.*$" contains=@Spell

syntax region zenString start=/"/ skip=/\\"/ end=/"/ contains=@Spell

" Integers, floats, and duration literals (number immediately followed by
" one of the recognized suffixes: ns us µs ms s m h). Longest suffixes are
" tried first so e.g. "30ms" doesn't get chopped at "m".
syntax match zenNumber "\<\d\+\%(\.\d\+\)\?\%(ns\|us\|µs\|ms\|s\|m\|h\)\?\>"

" Attributes, e.g. @primary, @unique, @validate(...), @foreign_key(user_id).
syntax match zenAttribute "@\w\+"

syntax match zenOperator "->"

highlight default link zenKeyword Keyword
highlight default link zenBoolean Boolean
highlight default link zenType Type
highlight default link zenLabel Identifier
highlight default link zenHTTPVerb Special
highlight default link zenComment Comment
highlight default link zenString String
highlight default link zenNumber Number
highlight default link zenAttribute PreProc
highlight default link zenOperator Operator

let b:current_syntax = "zen"
