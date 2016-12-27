#!/usr/bin/perl
use strict;
use warnings FATAL => 'all';
use lib '/root/scripts/utils';
use Mail;
use Config::IniFiles;
use File::ReadBackwards;
use Getopt::Long;
use DateTime::Format::Strptime;
Getopt::Long::Configure( "no_auto_abbrev" );

# Get the configuration option
my %opts;

GetOptions (
    \%opts,
    "config|c:s",   # This option indicates the configuration file to use
);

# Check if the configuration file exists
unless ($opts{'config'})
{
    print "No configuration file passed! Use the -c/--config option to pass a";
    print " configuration file to the script\n";
    exit 1;
}

unless (-r $opts{'config'})
{
    print "The configuration file passed (".$opts{'config'}.") is not ";
    print "readable. Please specify another one\n";
    exit 1;
}

my $parser = DateTime::Format::Strptime->new(
    pattern  => '%Y-%m-%dT%H:%M:%S',
    on_error => 'croak',
);
my $remote_backup_parser = DateTime::Format::Strptime->new(
    pattern  => '%m/%d/%y %H:%M%p',
    on_error => 'croak',
);
my $local_backup_parser = DateTime::Format::Strptime->new(
    pattern  => '%a %h %d %H:%M:%S %Z %Y',
    on_error => 'croak',
);
my $timezone = DateTime::TimeZone->new( name => 'local' );

my $config = Config::IniFiles->new ( -file => $opts{'config'} );
my $client_id = $config->val( "SMS", "ClientId" );
my $sender = $config->val( "SMS", "Sender" );
my $recipient = $config->val( "SMS", "Recipient" );
my $remoteBackupLogFile = $config->val( "Logfiles", "Remote" );
my $localBackupLogFile = $config->val( "Logfiles", "Local" );

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
    $subject .= "Memory ";
    $message .= `free -m`."\n\n";
    $message .= `top -o %MEM -n 1 -b`."\n";
}

if (!localBackupOk())
{
    $problem = 1;
    $subject .= "Local Backup ";
    $message .= `tail /var/log/backup.log`."\n";
    $message .= `tail /var/log/backup.log.1`."\n";
}

if (!crashplanRunning())
{
    $problem = 1;
    $subject .= "Crashplan ";
    $message .= "Crashplan is currently not running: "."\n";
    $message .= `ps aux | grep crashplan`."\n";
}

if (!remoteBackupOk())
{
    $problem = 1;
    $subject .= "Remote Backup ";
    $message .= `tail /usr/local/crashplan/log/backup_files.log.0`."\n";
    $message .= `tail /usr/local/crashplan/log/backup_files.log.0.1`."\n";
}

if ($problem == 1)
{
    my $mailer = Mail->new();
    my $message_to_send = "Subject: ".$subject." alert on patklaey.ch\n".$message;
    $mailer->send( $message_to_send );
    my $sms_message;
    if ($mailer->error())
    {
        $sms_message = "$subject alert on patklaey.ch, login to server! Sending mail failed: ".$mailer->error();
        if (needToSendSMS())
        {
            sendSMS( $sms_message, $sender, $recipient, $client_id );
        }
    } else
    {
        $sms_message = "There is an alert on patklaey.ch, check mail for more information";
        if (needToSendSMS())
        {
            sendSMS( $sms_message, $sender, $recipient, $client_id );
        }
    }
} else
{
    resetSMSTimout();
}

sub remoteBackupOk
{
    my $reverseFileReader = File::ReadBackwards->new( $remoteBackupLogFile ) or die "Can't read $remoteBackupLogFile $!";
    my $last_line;
    if (!defined( $last_line = $reverseFileReader->readline ))
    {
        $remoteBackupLogFile .= ".1";
        $reverseFileReader = File::ReadBackwards->new( $remoteBackupLogFile ) or die "Can't read $remoteBackupLogFile $!";
        if (!defined( $last_line = $reverseFileReader->readline ))
        {
            return 0;
        }
    }

    $last_line =~ m/^I\s(\d{1,2}\/\d{1,2}\/\d{1,2}\s\d\d:\d\d(AM|PM))\s42/;
    my $last_execute_date = $1;
    my $last_remote_finish = $remote_backup_parser->parse_datetime( $last_execute_date );
    $last_remote_finish->add( days => 1 );
    my $currentTime = DateTime->now( time_zone => $timezone );
    my $timeDiff = DateTime->compare( $last_remote_finish, $currentTime );
    $next_line = $reverseFileReader->readline;
    chomp( $next_line );
    if ($timeDiff >= 0)
    {
        return 1;
    }

    return 0;
}

sub crashplanRunning
{
    my $command = "ps aux | grep crashplan | grep -v grep | wc -l";
    my $command_output = `$command`;
    chomp( $command_output );
    return $command_output == 1;
}

sub localBackupOk
{
    my $reverseFileReader = File::ReadBackwards->new( $localBackupLogFile ) or die "Can't read $localBackupLogFile $!";
    my $last_line;
    if (!defined( $last_line = $reverseFileReader->readline ))
    {
        $localBackupLogFile .= ".1";
        $reverseFileReader = File::ReadBackwards->new( $localBackupLogFile ) or die "Can't read $localBackupLogFile $!";
        if (!defined( $last_line = $reverseFileReader->readline ))
        {
            return 0;
        }
    }

    if ($last_line !~ m/^#+$/)
    {
        return 0;
    }

    my $next_line = $reverseFileReader->readline;
    if (defined $next_line)
    {
        my $last_local_finish = $local_backup_parser->parse_datetime( $next_line );
        $last_local_finish->add( days => 1 );
        $last_local_finish->add( hours => 1 );
        my $currentTime = DateTime->now( time_zone => $timezone );
        my $timeDiff = DateTime->compare( $last_local_finish, $currentTime );
        $next_line = $reverseFileReader->readline;
        chomp( $next_line );
        if ($next_line eq "Backup finished" && $timeDiff >= 0)
        {
            return 1;
        }
    }
    return 0;
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
    system( 'curl -ikX POST -d "{\"outboundSMSMessageRequest\":{\"senderAddress\":\"tel:'.$sender.'\", \"address\":[\"tel:'.$recipient.'\"],\"outboundSMSTextMessage\":{\"message\":\"'.$message.'\"},\"clientCorrelator\":\"any id\"}}" -H "Content-Type:application/json" -H "Accept:application/json" -H "client_id: '.$client_id.'" https://api.swisscom.com/v1/messaging/sms/outbound/tel:'.$recipient.'/requests' );
    $config->setval( "SMS", "LastSent", DateTime->now( time_zone => $timezone ) );
    increaseSMSTimeout();
}

sub increaseSMSTimeout
{
    my $currentTimeout = $config->val( "SMS", "Timeout" );
    my $newTimeout;
    if ($currentTimeout == 0)
    {
        $newTimeout = 1;
    } else
    {
        $newTimeout = $currentTimeout * 2;
    }
    $config->setval( "SMS", "Timeout", $newTimeout );
    $config->RewriteConfig();
}

sub needToSendSMS
{
    my $lastSent = $config->val( "SMS", "LastSent" );
    if (!defined $lastSent || $lastSent eq "")
    {
        return 1;
    }

    my $lastSentTime = $parser->parse_datetime( $lastSent );
    my $currentTime = DateTime->now( time_zone => $timezone );
    my $timeout = $config->val( "SMS", "Timeout" );
    my $nextSendTime = $lastSentTime->add( hours => $timeout );
    if (DateTime->compare( $currentTime, $nextSendTime ) >= 0)
    {
        return 1;
    } else
    {
        return 0;
    }
}

sub resetSMSTimout
{
    if ($config->val( "SMS", "Timeout" ) != 0)
    {
        $config->setval( "SMS", "Timeout", 0 );
        $config->RewriteConfig();
    }
}