#!/usr/bin/perl
use strict;
use warnings FATAL => 'all';

use HTTP::Request;
use JSON::MaybeXS qw(decode_json);
use Config::IniFiles;
use File::Basename;
use LWP::UserAgent;

use lib '/root/scripts/utils';
use Mail;

my $dirname = dirname(__FILE__);
my $config = Config::IniFiles->new (-file => "$dirname/checkDockerImages.conf");

my $date = `date +"%Y-%m-%d %H:%M"`;
chomp($date);

my $userAgent = LWP::UserAgent->new;
my $page_size = 100;

my @reposToCheck = $config->val("Config", "Images");

my $mailMessage = "";

print "$date: Checking the following repos for newer versions: @reposToCheck\n";

foreach my $repository (@reposToCheck) {
    my $currentVersion = getCurrentVersion($repository);
    my $latestVersion = getLatestVersion($repository);
    if( defined $currentVersion && defined $latestVersion) {
        if($currentVersion eq $latestVersion) {
            print "$date: $repository has latest version installed ($currentVersion)\n";
        } else {
            my $message = "$repository has a newer version available ($latestVersion) than currently installed ($currentVersion)\n";
            print "$date: $message";
            $mailMessage .= $message;
        }
    }
    else {
        print "$date: $repository either current or latest version not defined\n"
    }
}

if($mailMessage ne "") {
    my $mailer = Mail->new();
    my $message_to_send = "Subject: New Docker Versions Available\n" . $mailMessage;
    $mailer->send($message_to_send);
}

sub getCurrentVersion {
    my $repo = shift;
    $repo =~ s/^library\///;
    # Exception for wordpress, replace the official version with mine
    if ( $repo eq "wordpress") {
        $repo = "patklaey/wordpress"
    }
    my @localVersions = qx(docker images $repo --format {{.Tag}} | grep -v latest);
    chomp(@localVersions);
    return $localVersions[0];
}


sub getLatestVersion {
    my $repo = shift;

    my @array = ();
    my $url = "https://registry.hub.docker.com/v2/repositories/$repo/tags?page_size=$page_size";

    do {
        my $request = HTTP::Request->new(GET => $url);
        my $response = $userAgent->request($request);
        my $hash = decode_json($response->content);

        push @array, grep {$_ =~ /^v?\d+\.\d+.\d+$/} map {$_->{name}} grep {grep {$_->{architecture} eq "arm"} @{$_->{images}}} @{$hash->{results}};

        $url = $hash->{next};

    } while ($url);
    my @sortedArray = sort version_sort @array;
    return $sortedArray[0];
}

sub version_sort {
    my @aVersion = split /\./, $a;
    my @bVersion = split /\./, $b;
    $aVersion[0] =~ /^v?(\d+)$/;
    $aVersion[0] = $1;
    $bVersion[0] =~ /^v?(\d+)$/;
    $bVersion[0] = $1;
    if( $aVersion[0] > $bVersion[0]) {
        return -1;
    } elsif( $aVersion[0] < $bVersion[0]) {
        return 1;
    } else {
        if( $aVersion[1] > $bVersion[1]) {
            return -1;
        } elsif( $aVersion[1] < $bVersion[1]) {
            return 1;
        } else {
            if( $aVersion[2] > $bVersion[2]) {
                return -1;
            } elsif( $aVersion[2] < $bVersion[2]) {
                return 1;
            } else {
                return 0;
            }
        }
    }
}
