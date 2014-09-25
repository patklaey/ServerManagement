#!/usr/bin/perl -w

use strict;
use warnings;
use LWP::UserAgent;
use Config::IniFiles;
use Getopt::Long;
Getopt::Long::Configure("no_auto_abbrev");

# Get the configuration option
my %opts;

GetOptions (
    \%opts,
    "help|h",       # This option will display a short help message.
    "config|c:s",   # This option indicates the configuration file to use
);

# Check if the configuration file exists
unless ( $opts{'config'} )
{
    print "No configuration file passed! Use the -c/--config option to pass a";
    print " configuration file to the script\n";
    exit 1;
}

unless ( -r $opts{'config'} )
{
    print "The configuration file passed (".$opts{'config'}.") is not ";
    print "readable. Please specify another one\n";
    exit 1;
}
 
# Get the configuration file
my $cfg = Config::IniFiles->new ( -file => $opts{'config'} );

# Read the configuration
my $logfile = $cfg->val("General","Logfile");
my $old_ip_file = $cfg->val("General","OldIpFile");

# Open the logfile
unless ( open( LOG, ">>$logfile" ) )
{
    print "Cannot open logfile $logfile!\n";
    exit 1;
}
 
# Create a new user agent (fake mozilla firefox to make the website
# think it is a real browser
my $ua = LWP::UserAgent->new(agent => "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:32.0) Gecko/20100101 Firefox/32.0");

# Get the date
my $date = `date +"%Y-%m-%dT%H:%M:%S"`;
chomp($date);

# And download the conthent from the website
my $response = $ua->get('http://whatismyip.pw');
 
# Parse the result
unless ( $response->is_success )
{
    print LOG "$date : Ipcheck failed! Cannot get content of www.whatismyip";
    print LOG ".pw\n";
    close LOG;
    exit 1;
}

my $content = $response->decoded_content;

$content =~ m/Your IP is\: (\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/;

my $current_ip = $1;

# Get the old ip address from the file
if ( -e $old_ip_file )
{
    open( OLD, $old_ip_file );
    my $old_ip = <OLD>;

    if ( $old_ip ne $current_ip )
    {
        # Send the mail
        system("echo 'Dear admin, The public IP of our home changed from $old_ip to $current_ip. Please update the bind configuration and switchplus nameserver in order to have a functional DNS. Cheers your ip-check script' | mutt -s 'Our Public IP changed to $current_ip' -- pat.klaey\@unifr.ch");
        open( NEW, ">$old_ip_file" );
	print NEW $current_ip;
        close NEW;
        print LOG "$date : IP changed from $old_ip to $current_ip. Mail sent";
        print LOG " to the administrator\n";
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
