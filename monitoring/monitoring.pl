#!/usr/bin/perl
use strict;
use warnings FATAL => 'all';
use lib '/root/scripts/utils';
use Mail;
use Config::IniFiles;
use File::ReadBackwards;
use Getopt::Long;
use DateTime::Format::Strptime;
Getopt::Long::Configure("no_auto_abbrev");
use JSON::MaybeXS qw(encode_json);

# Get the configuration option
my %opts;
my $date = `date +"%Y-%m-%d %H:%M"`;
chomp($date);

GetOptions (
    \%opts,
    "config|c:s",   # This option indicates the configuration file to use
);

# Check if the configuration file exists
unless ($opts{'config'}) {
    print "No configuration file passed! Use the -c/--config option to pass a";
    print " configuration file to the script\n";
    exit 1;
}

unless (-r $opts{'config'}) {
    print "The configuration file passed (" . $opts{'config'} . ") is not ";
    print "readable. Please specify another one\n";
    exit 1;
}

my $parser = DateTime::Format::Strptime->new(
    pattern  => '%Y-%m-%dT%H:%M:%S',
    on_error => 'undef',
);
my $remote_backup_parser = DateTime::Format::Strptime->new(
    pattern  => '%s',
    on_error => 'undef',
);
my $local_backup_parser = DateTime::Format::Strptime->new(
    pattern  => '%a %h %d %H:%M:%S %Z %Y',
    on_error => 'undef',
);
my $timezone = DateTime::TimeZone->new(name => 'local');
my $utc = DateTime::TimeZone::UTC->new;

my $config = Config::IniFiles->new (-file => $opts{'config'});
my $client_id = $config->val("SMS", "ClientId");
my $sender = $config->val("SMS", "Sender");
my $recipient = $config->val("SMS", "Recipient");
my $remoteBackupLogFileLocation = $config->val("Backup", "Remote");
my $localBackupLogFile = $config->val("Backup", "Local");

my $subject = "";
my $message = "Hello, there seems to be a problem on the server patklaey.ch:\n\n";
my $problem = 0;

my @load = getCpuUsage();
if ($load[0] > 1 || $load[1] > 0.75 || $load[2] > 0.5) {
    $problem = 1;
    $subject .= "CPU ";
    $message .= `top -n 1 -b` . "\n";
}

if (getFreeMemory() < 200000) {
    $problem = 1;
    $subject .= "Memory ";
    $message .= `free -m` . "\n\n";
    $message .= `top -o %MEM -n 1 -b` . "\n";
}

if (!localBackupOk()) {
    if( $config->val("Backup", "LocalSuccess") ) {
        $problem = 1;
        $subject .= "Local Backup ";
        $message .= `tail /var/log/backup.log` . "\n";
        $message .= `tail /var/log/backup.log.1` . "\n";
        $config->setval("Backup", "LocalSuccess", 0);
        $config->RewriteConfig();
    } else {
        print "$date: Local backup still not successful... Not sending notification as already notified\n";
    }
} else {
    $config->setval("Backup", "LocalSuccess", 1);
    $config->RewriteConfig();
}

if (!remoteBackupOk()) {
    if( $config->val("Backup", "RemoteSuccess") ) {
        $problem = 1;
        $subject .= "Remote Backup ";
        $message .= "Last remote backup was not successful or not run, please check the following log:\n";
        $message .= `ls -l $remoteBackupLogFileLocation | grep -v tar.gz | tail -n2` . "\n";
        $config->setval("Backup", "RemoteSuccess", 0);
        $config->RewriteConfig();
    } else {
        print "$date: Remote backup still not successful... Not sending notification as already notified\n";
    }
} else {
    $config->setval("Backup", "RemoteSuccess", 1);
    $config->RewriteConfig();
}

if ($problem == 1) {
    print "$date: Monitoring script detected the following problems: $subject, going to send mail and SMS if necessary\n";
    my $mailer = Mail->new();
    my $message_to_send = "Subject: " . $subject . " alert on patklaey.ch\n" . $message;
    $mailer->send($message_to_send);
    my $sms_message;
    if ($mailer->error()) {
        $sms_message = "$subject alert on patklaey.ch, login to server! Sending mail failed: " . $mailer->error();
        if (needToSendSMS()) {
            sendSMS($sms_message, $sender, $recipient, $client_id);
        }
    }
    else {
        $sms_message = "There is an alert on patklaey.ch, check mail for more information";
        if (needToSendSMS()) {
            sendSMS($sms_message, $sender, $recipient, $client_id);
        }
    }
}
else {
    resetSMSTimout();
    print "$date: Monitoring script has no problems detected, all smooth here!\n"
}

sub remoteBackupOk {
    my $hours_diff = 2;
    my $lastExecutionState = "";
    my $last_execute_date = "";
    my $lastLogFile = `ls -l $remoteBackupLogFileLocation | grep -v tar.gz | tail -n1 | awk '{print \$9}'`;
    if( $lastLogFile !~ m/^(\d+)_(\w+)_(\w+)$/) {
        print "$date: Something went wrong, last logfile not in expected format %timestamp%-%Success|Failure%-%Scheduled|Manual% but: $lastLogFile\n";
        return 0;
    } else {
        $lastExecutionState = $2;
        $last_execute_date = $1;
        if ($lastExecutionState eq "Running") {
            print "$date: Remote backup is still in progress, checking previous...\n";
            $lastLogFile = `ls -l $remoteBackupLogFileLocation | grep -v tar.gz | tail -n2 | head -n1 | awk '{print \$9}'`;
            $hours_diff = 4;
            if ($lastLogFile !~ m/^(\d+)_(\w+)_(\w+)$/) {
                print "$date: Something went wrong, logfile not in expected format %timestamp%-%Success|Failure%-%Scheduled|Manual% but: $lastLogFile\n";
                return 0;
            } else {
                $lastExecutionState = $2;
                $last_execute_date = $1;
            }
        }
    }

    if( $lastExecutionState ne "Success") {
        return 0;
    }

    my $last_remote_finish = $remote_backup_parser->parse_datetime($last_execute_date);
    $last_remote_finish->add(hours => $hours_diff);
    my $currentTimeUTC = DateTime->now(time_zone => $utc);
    my $timeDiff = DateTime->compare($last_remote_finish, $currentTimeUTC);
    if ($timeDiff >= 0) {
        return 1;
    }
    return 0;
}

sub localBackupOk {
    my $reverseFileReader = File::ReadBackwards->new($localBackupLogFile) or die "Can't read $localBackupLogFile $!";
    my $last_line;
    if (!defined($last_line = $reverseFileReader->readline)) {
        $localBackupLogFile .= ".1";
        $reverseFileReader = File::ReadBackwards->new($localBackupLogFile) or die "Can't read $localBackupLogFile $!";
        if (!defined($last_line = $reverseFileReader->readline)) {
            return 0;
        }
    }

    if ($last_line !~ m/^#+$/) {
        return 0;
    }

    my $backup_success = 0;
    my $next_line = $reverseFileReader->readline;
    if (defined $next_line) {
        my $last_local_finish = $local_backup_parser->parse_datetime($next_line);
        if( ! defined $last_local_finish || defined $local_backup_parser->errmsg ){
            print "Failed to parse datetime ($next_line), something is fishy\n";
            return 0;
        }
        $last_local_finish->add(days => 1);
        $last_local_finish->add(hours => 1);
        my $currentTime = DateTime->now(time_zone => $timezone);
        my $timeDiff = DateTime->compare($last_local_finish, $currentTime);
        $next_line = $reverseFileReader->readline;
        chomp($next_line);
        if ($next_line eq "Backup finished" && $timeDiff >= 0) {
            $backup_success = 1;
        }
    }

    # Check the space on the backup disk
    my $enough_space = 0;
    $next_line = $reverseFileReader->readline;
    my @headers = qw(name size used free capacity mount);
    my %info;
    while( defined($next_line) ){
        if( $next_line =~ m/^\/.*\/mnt\/backup/ ){
            @info{@headers} = split /\s+/, $next_line;
            my $decimal_free = percentageToDecimal($info{capacity});
            if( $decimal_free < 0.85 ) {
                $enough_space = 1;
            } else {
                print "$date: Low backup storage: $decimal_free\n"
            }
            last;
        }
        # To avoid going back too far
        if( $next_line =~ m/^Filesystem/ ){
            last;
        }
        $next_line = $reverseFileReader->readline;
    }
    return $backup_success && $enough_space;
}

sub percentageToDecimal {
    my $percentage = shift;
    $percentage =~ s{%}{};
    return $percentage / 100;
}

sub getCpuUsage {
    open(LOADAVG, '/proc/loadavg') or return ();
    my @loadavg = <LOADAVG>;
    close LOADAVG;
    @loadavg = split(" ", $loadavg[0]);
    return ($loadavg[0], $loadavg[1], $loadavg[2]);
}

sub getFreeMemory {
    open(FREEMEM, '/proc/meminfo') or return ();
    my @meminfo = <FREEMEM>;
    close FREEMEM;

    my $free_mem = 0;
    foreach my $meminfo (@meminfo) {
        if ($meminfo =~ m/^MemFree:\s+(\d+)\skB$/ || $meminfo =~ m/^Cached:\s+(\d+)\skB$/) {
            $free_mem += $1;
        }
    }
    return $free_mem;
}

sub sendSMS {
    my ($smsMessage, $smsSender, $smsRecipient, $smsClientId) = @_;

    my $body = {
        "from" => $smsSender,
        "to" => $smsRecipient,
        "text" => $smsMessage
    };

    my $bodyAsString = encode_json($body);

    system('curl -ikX POST -d \'' . $bodyAsString . '\' -H "SCS-Version:2" -H "Content-Type:application/json" -H "Accept:application/json" -H "client_id: ' . $smsClientId . '" https://api.swisscom.com/messaging/sms');
    $config->setval("SMS", "LastSent", DateTime->now(time_zone => $timezone));
    increaseSMSTimeout();
}

sub increaseSMSTimeout {
    my $currentTimeout = $config->val("SMS", "Timeout");
    my $newTimeout;
    if ($currentTimeout == 0) {
        $newTimeout = 1;
    }
    else {
        $newTimeout = $currentTimeout * 2;
    }
    $config->setval("SMS", "Timeout", $newTimeout);
    $config->RewriteConfig();
}

sub needToSendSMS {
    my $lastSent = $config->val("SMS", "LastSent");
    if (!defined $lastSent || $lastSent eq "") {
        return 1;
    }

    my $lastSentTime = $parser->parse_datetime($lastSent);
    my $currentTime = DateTime->now(time_zone => $timezone);
    my $timeout = $config->val("SMS", "Timeout");
    my $nextSendTime = $lastSentTime->add(hours => $timeout);
    if (DateTime->compare($currentTime, $nextSendTime) >= 0) {
        return 1;
    }
    else {
        return 0;
    }
}

sub resetSMSTimout {
    if ($config->val("SMS", "Timeout") != 0) {
        $config->setval("SMS", "Timeout", 0);
        $config->RewriteConfig();
    }
}
