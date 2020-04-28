# Tracing in Onet

## TLDR

Import the tracing service:

```
import _ go.dedis.ch/onet/v3/tracing/service
```

Sign up on https://honeycomb.io and get an API key.
Create a dataset on honeycomb.io.
Run your binary like this:

```
HONEYCOMB_API_KEY=api_key:dataset ./conode
```

Start tracing!

## Why

While logging is quite important in setting up and running a system, it has
 its limits:
- it gets garbled when multiple nodes run at the same time
- only prepared messages can be seen
- difficult to filter and chose which messages are printed

This is why tracing exists, which I learnt from Charity Majors:
 https://twitter.com/mipsytipsy
With her great name, good writing style, calling out people while still
 listening to suggestions, she convinced me that tracing is needed, not only
 logging, monitoring, ..., but tracing.
The biggest advantage of tracing is to be able to look at collected `traces`
 that contain one or more `spans`.
Each trace tells a little story about what's happening in the code.
Usually it's a user interaction, but for onet it also makes sense to have
 protocols behaviour's traced.

## How

In order not to have to rewrite a lot of code, this tracing module uses the
 `onet/log` package to simulate the tracing.
By signing up as a `Logger`, this package gets informed of every `log.*` call.
It looks at the stack that lead up to that call and decides to which trace
 this `log` belongs.
This allows it to use all the `log.*` calls already in the code and to start
 directly tracing.
In order to detect trace-starts, there is a list of methods that indicate a
 new trace.
 
As go doesn't support tracing of which go-routines are started by which other
 go-routine, the tracing mechanism needs some manual support:
`log.TraceID` allows different go-routines to register to the tracing service
 as spans belonging together.
This method is used to link different parts of the protocol together, by
 using the `onet.Token.RoundID` as unique identifier.
This should even work cross-node.
 
## Add your own traces

There is a good chance that it will work out-of-the box.
If you want to add your own methods where a trace should start, simply add
 them to the environment:
 
```
export TRACING_ENTRY_POINTS="github.com/org/repo/pkg.method"
export TRACING_DONE_MSGS="done with tracing"
HONEYCOMB_API_KEY=api_key:dataset ./conode
```

If you're wondering what traces are available, you can set

```
export TRACING_PRINT_SINGLE_SPANS=10
```

This will output stack-traces with length `10` on all unregistered spans.
The outputted lines can be added to `TRACING_ENTRY_POINTS`, as well as a
 meaningful done message to `TRACING_DONE_MSGS`.
And voilà, your own tracing is enabled.

## Unknown traces

Some of the traces will not be found.
If you set `TRACING_CREATE_SINGLE_SPANS=true`, these unknown traces will be
 created as traces with a single span.
This is not pretty, but might turn out useful sometimes.

## Debugging

As loggers cannot use onet/log, you can set the following environment
 variable to output some debugging information about what's happening:
 
```
export TRACING_DEBUG=true 
```

