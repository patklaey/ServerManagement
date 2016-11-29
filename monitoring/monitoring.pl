#!/usr/bin/perl
use strict;
use warnings FATAL => 'all';
use lib '/root/scripts/utils';
use Mail;
use Config::IniFiles;

my $config = Config::IniFiles->new ( -file => './monitoring.conf' );
my $client_id = $config->val( "Sms", "ClientId" );
my $sender = $config->val( "Sms", "Sender" );
my $recipient = $config->val( "Sms", "Recipient" );
my $subject = "";
my $message = "Hello, there seems to be a problem on the server patklaey.ch:\n\n";
my $problem = 0;

my @load = getCpuUsage();
if ($load[0] > 1 || $load[1] > 0.75 || $load[2] > 0.5)
{
    $problem = 1;
    $subject .= "CPU ";
    $message .= `top -n 1 -b`."\n";
}

if (getFreeMemory() < 200000)
{
    $problem = 1;
    $subject = "Memory ";
    $message .= `free -m`."\n\n";
    $message .= `top -o %MEM -n 1 -b`."\n";
}

if ($problem == 1)
{
    my $mailer = Mail->new();
    my $message_to_send = "Subject: ".$subject." alert on patklaey.ch\n".$message;
    $mailer->send( $message_to_send );
    my $sms_message = "Error";
    if ($mailer->error())
    {
        $sms_message = "$subject alert on patklaey.ch, login to server! Sending mail failed: ".$mailer->error();
        sendSMS( $sms_message, $sender, $recipient, $client_id );
    } else
    {
        $sms_message = "There is an alert on patklaey.ch, check mail for more information";
        sendSMS( $sms_message, $sender, $recipient, $client_id );
    }
}

sub getCpuUsage
{
    open( LOADAVG, '/proc/loadavg' ) or return ();
    my @loadavg = <LOADAVG>;
    close LOADAVG;
    @loadavg = split( " ", $loadavg[0] );
    return ($loadavg[0], $loadavg[1], $loadavg[2]);
}

sub getFreeMemory
{
    open( FREEMEM, '/proc/meminfo' ) or return ();
    my @meminfo = <FREEMEM>;
    close FREEMEM;

    my $free_mem = 0;
    foreach my $meminfo (@meminfo)
    {
        if ($meminfo =~ m/^MemFree:\s+(\d+)\skB$/ || $meminfo =~ m/^Cached:\s+(\d+)\skB$/)
        {
            $free_mem += $1;
        }
    }
    return $free_mem;
}

sub sendSMS
{
    my ($message, $sender, $recipient, $client_id) = @_;
    system( 'curl -ikX POST -d "{\"outboundSMSMessageRequest\":{\"senderAddress\":\"tel:'.$sender.'\", \"address\":[\"tel:'.$recipient.'\"],\"outboundSMSTextMessage\":{\"message\":\"'.$message.'\"},\"clientCorrelator\":\"any id\"}}" -H "Content-Type:application/json" -H "Accept:application/json" -H "client_id: '.$client_id.'" https://api.swisscom.com/v1/messaging/sms/outbound/tel:'.$receipient.'/requests' );
}