/*
Package interpolate provides string interpolation from environment variables as
per the Compose specification. See [Interpolation specification] for the
authoritative source of truth our implementation should adhere to.

This interpolation uses a Bash-like syntax, both in so-called “unbraced” and
“braced” syntax:

	$FOO
	${FOO}

But unlike Bash, interpolation can be nested:

	${FOO:-${BAR}}

The following subsitutions are supported, described below.

# Default

The following substitution

	${VARIABLE:-default}

evaluates to “default” if VARIABLE is unset or empty. In contrast,

	${VARIABLE-default}

evaluates to default only if VARIABLE is unset, but not if it is empty.

# Error

The following substitution

	${VARIABLE:?err}

exits with an error message containing err if VARIABLE is unset or empty. In
constrast,

	${VARIABLE?err}

exits with an error message containing err only if VARIABLE is unset, but not if
it is empty.

# Replacement

In addition to the Composer spec-defined default and error interpolations, this
package additionally supports the [Docker Compose Interpolation] of alternative
values:

	${VARIABLE:+replacement}

replaces with replacement if VARIABLE is set and non-empty, otherwise empty. In
contrast,

	${VARIABLE+replacement}

replaces with replacement if VARIABLE is set, otherwise empty, but not if it is
empty.

# Implementation Note

While the “Compose specification” Github organization provides a [Compose Spec
reference interpolation implementation], this module provides its own
implementation. In particular, this interpolation is implemented as a dedicated
parser instead of using regular expressions. A dedicated parser implementation
is probably much more straightforward to carry out, understand, and maintain
than regular expressions that really don't work well in the face of recursive
interpolation where they need dodgy parsing helpers anyway.

[Interpolation specification]: https://github.com/compose-spec/compose-spec/blob/main/12-interpolation.md
[Docker Compose Interpolation]: https://docs.docker.com/reference/compose-file/interpolation/
[Compose Spec reference interpolation implementation]: https://github.com/compose-spec/compose-go/tree/main/interpolation
*/
package interpolate
