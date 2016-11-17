#!/usr/bin/perl
use strict;
use warnings FATAL => 'all';
use lib '/root/scripts/utils';
use Mail;

my @load = getCpuUsage();
if ($load[0] > 1 || $load[1] > 0.75 || $load[2] > 0.5)
{
    my $mailer = Mail->new();
    my $message = `top -n 1 -b`;
    $message = "Subject: Alert on patklaey.ch\nHello, there seems to be a problem on the server patklaey.ch:\n\n" . $message;
    $mailer->send($message);
    if ( $mailer->error() )
    {
        print "Problem, sending mail failed:". $mailer->error()."\n";
    }
}

sub getCpuUsage
{
    open(LOADAVG, '/proc/loadavg') or return ();
    my @loadavg = <LOADAVG>;
    close LOADAVG;
    @loadavg = split(" ", $loadavg[0]);
    return ($loadavg[0],$loadavg[1],$loadavg[2]);
}