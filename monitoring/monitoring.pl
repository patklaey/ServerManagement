#!/usr/bin/perl
use strict;
use warnings FATAL => 'all';

my @load = getCpuUsage();
if ($load[0] > 1 || $load[1] > 0.75 || $load[2] > 0.5)
{
    print "oups, we have high load: @load";
} else
{
    print "Everything OK, we have low load: @load";
}

sub getCpuUsage
{
    open(LOADAVG, '/proc/loadavg') or return ();
    my @loadavg = <LOADAVG>;
    close LOADAVG;
    @loadavg = split(" ", $loadavg[0]);
    return ($loadavg[0],$loadavg[1],$loadavg[2]);
}