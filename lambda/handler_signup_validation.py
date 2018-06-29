import os
import json
import boto3

def handler(event, context):
    s3 = boto3.client('s3')

    if 'BUCKET_NAME' in os.environ:
        bucket_name = os.environ['BUCKET_NAME']
    else:
        return {
            'statusCode': 500,
            'body': json.dumps({
                'message': 'BUCKET_NAME is not defined!'
            }),
        }


    s3_get_response = s3.get_object(
        Bucket=bucket_name,
        Key='user_whitelist.json',
    )
    whitelist = json.loads(s3_get_response['Body'])
    domains = whitelist['domains']
    addresses = whitelist['addresses']
    if len(domains) == 0 and len(addresses) == 0:
        # If the whitelist is empty, allow everyone!
        return event

    email_addr = event['request']['userAttributes']['email']
    email_parts = email_addr.split('@')
    if len(email_parts) > 2:
        # GET OUT
        raise Exception('Bad email address')
    for domain in domains:
        if email_parts[1] == domain:
            return event
    if email_addr in set(addresses):
        return event

    raise Exception('Unauthorized email address')
