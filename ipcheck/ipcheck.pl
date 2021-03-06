#!/usr/bin/perl -w

use strict;
use warnings;
use Config::IniFiles;
use Getopt::Long;
Getopt::Long::Configure( "no_auto_abbrev" );

use lib '/root/scripts/utils';
use Mail;

# Get the configuration option
my %opts;

GetOptions (
    \%opts,
    "help|h", # This option will display a short help message.
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

# Get the configuration file
my $cfg = Config::IniFiles->new ( -file => $opts{'config'} );

# Read the configuration
my $logfile = $cfg->val( "General", "Logfile" );
my $old_ip_file = $cfg->val( "General", "OldIpFile" );
my $mail_address = $cfg->val( "Mail", "To" );

# Open the logfile
unless (open( LOG, ">>$logfile" ))
{
    print "Cannot open logfile $logfile!\n";
    exit 1;
}


# Get the date
my $date = `date +"%Y-%m-%dT%H:%M:%S"`;
chomp( $date );

# Get the current IP
my $current_ip = `nslookup patklaey.internet-box.ch | awk -F': ' 'NR==6 { print \$2 } '`;
chomp($current_ip);

if( ! defined $current_ip || $current_ip eq "" ) {
    my $nslookup = `nslookup patklaey.internet-box.ch`;
    print LOG "$date : IP could not be retrieved. Current nslookup result is: $nslookup";
    close LOG;
    exit 0;
}

# Get the old ip address from the file
if (-e $old_ip_file)
{
    open( OLD, $old_ip_file );
    my $old_ip = <OLD>;

    if ($old_ip ne $current_ip)
    {
        # Send the mail
        my $mailer = Mail->new();
        $mailer->setTo($mail_address);
        my $message = "Dear admin, The public IP of our home changed from $old_ip to $current_ip. Please update the bind configuration and switchplus nameserver in order to have a functional DNS.\n\nCheers your ip-check script";
        $message = "Subject: Our Public IP changed to $current_ip\n".$message;
        open( NEW, ">$old_ip_file" );
        print NEW $current_ip;
        close NEW;
        $mailer->send( $message );
        if ($mailer->error())
        {
            print LOG "$date : Mail could not be sent: ".$mailer->error()."\n";
        } else
        {
            print LOG "$date : IP changed from $old_ip to $current_ip. Mail sent";
            print LOG " to the administrator (".join( ",", $mailer->getTo() ).")\n";
        }
    } else
    {
        print LOG "$date : Ipcheck successful, still the same ip: $current_ip";
        print LOG "\n";
    }

} else
{
    open( OLD, ">$old_ip_file" );
    print OLD $current_ip;
    close OLD;
}

close LOG;
exit 0;
