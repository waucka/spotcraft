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

    new_config = json.loads(event['body'])
    server_name = event['pathParameters']['serverName']

    s3_get_response = s3.get_object(
        Bucket=bucket_name,
        Key="servers/{server_name}/config.json".format(server_name=server_name),
    )
    old_config = json.loads(s3_get_response['Body'])
    merged_config = {**old_config, **new_config}

    s3_put_response = s3.put_object(
        Bucket=bucket_name,
        Key="servers/{server_name}/config.json".format(server_name=server_name),
        Body=json.dumps(merged_config),
    )

    return {
        'statusCode': 200,
        'body': json.dumps(servers),
    }
