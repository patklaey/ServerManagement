#!/usr/bin/perl
use strict;
use warnings FATAL => 'all';

use LWP::Curl;
use JSON::MaybeXS qw(decode_json);
use Config::IniFiles;
use File::Basename;

my $dirname = dirname(__FILE__);
my $config = Config::IniFiles->new (-file => "$dirname/checkDockerImages.conf");

my $date = `date +"%Y-%m-%d %H:%M"`;
chomp($date);

my $lwpcurl = LWP::Curl->new();
my $page_size = 100;

my @reposToCheck = $config->val("Config", "Images");

my $mailMessage = "";

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
    my $message_to_send = "Subject: New Docker Versions Available" . $mailMessage;
    $mailer->send($message_to_send);
}

sub getCurrentVersion {
    my $repo = shift;
    $repo =~ s/^library\///;
    my @localVersions = qx(docker images $repo --format {{.Tag}} | grep -v latest);
    chomp(@localVersions);
    return $localVersions[0];
}


sub getLatestVersion {
    my $repo = shift;

    my @array = ();
    my $url = "https://registry.hub.docker.com/v2/repositories/$repo/tags?page_size=$page_size";

    do {
        my $content = $lwpcurl->get($url);

        my $hash = decode_json($content);

        push @array, grep {$_ =~ /^\d+\.\d+.\d+$/} map {$_->{name}} grep {grep {$_->{architecture} eq "arm"} @{$_->{images}}} @{$hash->{results}};

        $url = $hash->{next};

    } while ($url);
    return $array[0];
}

