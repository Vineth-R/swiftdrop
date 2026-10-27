package config

import (
	"errors"
	"net/url"
	"strings"
)

func ValidateAMQPURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("AMQP URL must not be empty")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("AMQP URL is invalid")
	}
	if !strings.EqualFold(parsedURL.Scheme, "amqp") && !strings.EqualFold(parsedURL.Scheme, "amqps") {
		return errors.New("AMQP URL must use the amqp or amqps scheme")
	}
	if parsedURL.Host == "" {
		return errors.New("AMQP URL must include a host")
	}
	return nil
}
