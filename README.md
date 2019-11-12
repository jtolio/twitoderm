# Twitoderm CQ

Some of the best minds of 50 years ago spent their time putting people on the moon.
We arguably have even more "great minds" now working on getting people to look at ads.
Specifically, huge firehoses of money are spent to addict people to their feeds and
smart phones. People (including me!) spend countless hours scrolling through statuses
and updates, all for the sake of interspersed advertisements.

The "great minds" of our time are very good at hooking me, anyway, and driving
the brain chemicals so that I can't put my phone down.

I can't just ban Twitter and Reddit though. I need them for work.

Twitoderm CQ seeks to put an end to social media feed addiction by making
specifically addicting sites frustrating to load.

## How it works

Twitoderm CQ is a DNS and TCP proxy. Users point their DNS client configuration
at a running Twitoderm server, and Twitoderm just proxies through all DNS requests,
unless the domain name is one of the configured bad sites. A configured bad site
is returned an A record with TTL 5 that points back to Twitoderm.

Twitoderm then uses TLS SNI and HTTP/1.1 Host headers to proxy the TCP traffic
to and from the real destination, but much slower and with connection delays.

Ideally, this is enough frustration to break my dopamine loop.

### Update

My DNS forwarder has some pretty significant bugs. I'm currently using
[dnsmasq](http://www.thekelleys.org.uk/dnsmasq/doc.html)
for DNS, but am otherwise using Twitoderm unmodified. If someone wants
to fix my DNS forwarding I would be delighted.

## Licence

Copyright (C) 2019, JT Olio
Licensed under Apache v2. See LICENSE for more info
