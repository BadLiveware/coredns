# gcp_instances

## Name

*gcp_instances* - enables serving templated dns records from gcp compute instances

## Description

The *gcp_instances* plugin is useful for convention over configuration when using host records, or transitioning from using the gcp internal DNS to a custom DNS solution.


## Syntax

~~~ txt
gcp_instances  {
    template    <Go template string>
    project     <A gcp project>
    credentials [FILENAME]
    fallthrough [ZONES...]
}
~~~

*   **TEMPLATE** A go template string(including sprig functions) to use for templating DNS records for compute instances

*   **PROJECT** the project ID of the Google Cloud project.

*   `credentials` is used for reading the credential file from **FILENAME** (normally a .json file).
    This field is optional. If this field is not provided then authentication will be done automatically,
    e.g., through environmental variable `GOOGLE_APPLICATION_CREDENTIALS`. Please see
    Google Cloud's [authentication method](https://cloud.google.com/docs/authentication) for more details.

*   `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
    If **[ZONES...]** is omitted, then fallthrough happens for all zones for which the plugin is
    authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then
    only queries for those zones will be subject to fallthrough.

## Examples

Enable clouddns with implicit GCP credentials and resolve CNAMEs via 10.0.0.1:

~~~ txt
. {
    gcp_instances {
        template "{{ .Name }}.{{ .Project }}.example.org."
    }
    forward . 10.0.0.1
}
~~~

Mimic googles internal DNS with an option to override the .Name property with a label on the compute instance
~~~ txt
.internal {
    gcp_instances {
        template "{{ .Labels.dns | default .Name }}.{{ .Zone }}.c.{{ .Project }}.internal."
    }
}
~~~