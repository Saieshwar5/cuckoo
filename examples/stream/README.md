# stream

An agent that replies a word at a time. It proves streaming end to end: the
person sees an empty bubble appear, fill up, and become final.

Set up an agent with a socket binding exactly as in [`../echo`](../echo),
then:

```bash
CUCKOO_SECRET=bnd_sec_... python stream.py
```

Kill it mid-sentence and the hub finishes the message for it, marked
truncated, thirty seconds after the last word.
