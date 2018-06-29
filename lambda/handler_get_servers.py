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

    s3_list_response = s3.list_objects_v2(
        Bucket=bucket_name,
        Prefix='servers/',
        Delimiter='/',
    )

    servers = {}
    for subfolder in s3_list_response['CommonPrefixes']:
        prefix = subfolder['Prefix']
        server_name = prefix.split('/')[1]
        key = "{prefix}/config.json".format(prefix=prefix)
        s3_get_response = s3.get_object(
            Bucket=bucket_name,
            Key=key,
        )
        servers[server_name] = json.loads(s3_get_response['Body'])

    return {
        'statusCode': 200,
        'body': json.dumps(servers),
    }
