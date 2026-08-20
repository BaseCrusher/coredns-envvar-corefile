# coredns-envvar-corefile

Generates a CoreDNS [Corefile](https://coredns.io/manual/toc/#configuration)
from environment variables and prints it to stdout.

## Motivation

Everything about a container is usually configured through environment
variables — except CoreDNS. There you still have to author a Corefile and mount
it in (ConfigMap, bind mount, baked-in layer): a separate file to template,
ship, and keep in sync with the rest of your config.

This tool closes that gap. Describe the whole Corefile with
`COREDNS_`-prefixed environment variables, just like everything else, and let
the tool generate the file at startup. No Corefile to mount, no config split
across two mechanisms.

## How it works

Every variable starts with `COREDNS_`, followed by a **group** name. Each group
becomes one server block. Group names may not contain `_`.

| Env var | Meaning |
|---|---|
| `COREDNS_<GROUP>_ZONE` | zone for the block header (**required**) |
| `COREDNS_<GROUP>_PORT` | port for the block header (defaults to `53`) |
| `COREDNS_<GROUP>__<DIRECTIVE>` | a directive inside the block |
| `COREDNS_<GROUP>__<DIRECTIVE>__<SUB>` | nested one level deeper; add more `__` to go deeper still |

- **Single `_`** separates the group from a top-level header field (`ZONE`, `PORT`).
- **Double `__`** separates each level of directives inside the block. Every
  extra `__` opens another nested `{ ... }`.
- The value of a variable is placed inline after the directive. A directive with
  no value (e.g. `log`) uses an empty value: `COREDNS_MYZONE__log=`.
- Directive names are used **exactly as written** — case is preserved, so
  `optionOne` stays camelCase.
- A group without a `ZONE` is skipped. Blocks and directives are emitted in
  sorted order for deterministic output.

### Repeating a directive

Each directive name must be unique, so to emit the same directive more than once
under one parent, add a **trailing `_<digits>`** index. It (and its `_`) is
dropped before rendering, so `file_1` and `file_2` both render `file`. Names are
string-sorted, so pad the index (`_01`…`_12`) if you need more than 9 in order.

A **trailing `_` after the digits** escapes them into the name instead — `ip6_2_`
renders `ip6_2`, not `ip6` repeated.

### Writing `@`

Env-var names can't contain `@`, so **`_AT_`** renders as `@`. It is replaced
*before* the `__` split, so it composes with everything above: `_AT_` on its own
is `@`, `_AT__1` / `_AT__2` are `@` repeated (the token plus a `_1` index), and
`_AT___child` nests (`@ { child }`). That's enough to express a zone-record block:

```sh
export COREDNS_PROD_ZONE=example.com
export COREDNS_PROD__records___AT__1='60 IN SOA ns1.example.com. admin.example.com. 2025073111 3600 300 2419200 300'
export COREDNS_PROD__records___AT__2='60 IN NS ns1.example.com.'
export COREDNS_PROD__records___AT__3='60 IN A 192.0.2.1'
export COREDNS_PROD__records___AT__4='60 IN A 192.0.2.2'
export COREDNS_PROD__records___AT__5='60 IN A 192.0.2.3'
```

→

```
example.com:53 {
    records {
        @ 60 IN SOA ns1.example.com. admin.example.com. 2025073111 3600 300 2419200 300
        @ 60 IN NS ns1.example.com.
        @ 60 IN A 192.0.2.1
        @ 60 IN A 192.0.2.2
        @ 60 IN A 192.0.2.3
    }
}
```

## Example

```sh
export COREDNS_MYZONE_ZONE=example.org
export COREDNS_MYZONE__fancy_plugin=test
export COREDNS_MYZONE__fancy_plugin__optionOne='value 1'
export COREDNS_MYZONE__fancy_plugin__optionTwo='value 2'
export COREDNS_MYZONE__fancy_plugin__flagEnabled=
export COREDNS_MYZONE__file=db.example.org
```

Running the tool prints:

```
example.org:53 {
    fancy_plugin test {
        flagEnabled
        optionOne value 1
        optionTwo value 2
    }
    file db.example.org
}
```

Multiple groups produce multiple server blocks.

## Usage

```sh
go build -o corefile-gen .
./corefile-gen /etc/coredns/Corefile   # write to a file
./corefile-gen                         # or print to stdout
```

Writing the file directly means no shell redirection is needed, so it works in
a distroless image — e.g. as an init container that writes the Corefile into a
volume shared with CoreDNS:

```yaml
command: ["/corefile-gen", "/etc/coredns/Corefile"]
```
