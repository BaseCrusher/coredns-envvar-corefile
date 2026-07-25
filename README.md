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
