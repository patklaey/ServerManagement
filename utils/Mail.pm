#!/usr/bin/perl

package Mail;

use strict;
use warnings;

use Net::SMTPS;
use POSIX;


use constant
{
    TRUE    => 1,
    FALSE   => 0,
    
    DEFAULT_PORT => 25,
    
    SUCCESS => 0,
    UNKNOWN_ERROR => 1,
    NO_HOST => 2,
    CANNOT_SEND_MAIL => 3,
    NO_RECIPIENT_SET => 4,
    NO_SENDER_SET => 5,
    
};

sub new
{
    my $class = shift;
    my $self = {};

    my %parameters = @_;
    
    # Error and error code field are both undefined when constructig the object
    $self->{error} = undef;
    $self->{error_code} = 0;
    
    # Set up
    $self->{host} = $parameters{host} || undef;
    $self->{port} = $parameters{port} || DEFAULT_PORT;
    $self->{user} = $parameters{user} || undef;
    $self->{password} = $parameters{password} || undef;
    $self->{to} = $parameters{to} || [];
    $self->{from} = $parameters{from} || undef;

    # Check if host is defined
    unless ( $self->{host} )
    {
        $self->{error} = "No mail host defined, cannot create mail object "
                        ."without mail host";
        $self->{error_code} = NO_HOST;
        bless( $self, $class );
        return $self;
    }
 
    # Create the mail object using Net::SMTPS
    my $mailer = new Net::SMTPS( $self->{host},
                                 Port => $self->{port},
                               );
                               
    # Check if username and password are passed, if yes, authenticate
    if ( $parameters{user} && $parameters{password} )
    {
        $mailer->auth( $parameters{user}, $parameters{password}, 'LOGIN' );
    }
    
    # Assing the mailer
    $self->{mailer} = $mailer;
                                    
    bless( $self, $class );
    return $self;
}

sub error
{
    my $self = shift;
    
    # If the error field is set, we had an error so return it. Otherwise return
    # false
    defined $self->{error} ? return $self->{error} : return FALSE;

}

sub send
{
    
    my $self = shift;
    my $message = shift;
    
    # Test if at least one receipient is set
    unless ( $self->getTo() > 0 )
    {
        $self->{error} = "No recipient set. Please set at least one recipient "
                        ."using \$mail->setTo( \@to )";
        return $self->{error_code} = NO_RECIPIENT_SET;
    }
    
    # Check if sender is set
    unless ( $self->getFrom() )
    {
        $self->{error} = "No sender set. Please set at least one sender using "
                        ."\$mail->setFrom( \$from_address )";
        return $self->{error_code} = NO_SENDER_SET;
    }
    
    # Get the date in the correct locale:
    # Set the LC_TIME to en_US.UTF-8 but first save the actual LC_TIME
    # to be able to restore it later
    my $old_LC = setlocale(LC_TIME);
    setlocale(LC_TIME,"en_US.UTF-8");
    # get the date string
    my $date = strftime("%d %b %Y %H:%M:%S %z", localtime() );
    setlocale(LC_TIME,$old_LC);
    
    # Send all headers and the message
    $self->{mailer}->mail($self->{user} );
    $self->{mailer}->to( $self->getTo() );
    $self->{mailer}->data;
    $self->{mailer}->datasend("From: ".$self->getFrom()."\n");
    $self->{mailer}->datasend("To: ".join(",",$self->getTo())."\n");
    $self->{mailer}->datasend("Content-Type: text/plain; charset=UTF-8\n");
    $self->{mailer}->datasend("Date: $date\n");
    $self->{mailer}->datasend($message);
    
    # Finally send the dataend command to tell the server that no more data is 
    # beeing sent.
    $self->{mailer}->dataend;
    
    # Check for the message returned by the server
    my $mesg = $self->{mailer}->message();
    
    # Check if everything was ok
    unless( $mesg =~ m/2\.0\.0\sOk\:\squeued\sas/i )
    {
        $self->{mailer}->quit;
        $self->{error} = "Cannot send mail: $mesg";
        return $self->{error_code} = CANNOT_SEND_MAIL;
    }
    
    # Disconnect from the server
    $self->{mailer}->quit;
    
    return SUCCESS;
    
}

sub setTo
{
    my $self = shift;
    $self->{to} = \@_;
}

sub setFrom
{
    my $self = shift;
    $self->{from} = shift;
}

sub getTo
{
    my $self = shift;
    return @{$self->{to}};
}

sub getFrom
{
    my $self = shift;
    return $self->{from};
}

1;

__END__
