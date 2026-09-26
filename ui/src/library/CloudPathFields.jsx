import React, { useCallback, useMemo, useState } from 'react'
import {
  FormDataConsumer,
  RadioButtonGroupInput,
  TextInput,
  required,
  useNotify,
  useTranslate,
} from 'react-admin'
import { Box, Button, Typography } from '@material-ui/core'
import {
  SOURCE_TYPES,
  buildCloudPath,
  isCloudPath,
  parseCloudPath,
} from './cloudPath'

const authHeaders = () => ({
  'X-ND-Authorization': `Bearer ${localStorage.getItem('token')}`,
  'Content-Type': 'application/json',
})

/**
 * The "source" part of the library form: switch between a local folder (exactly the
 * behaviour the form always had) and a cloud folder (OpenList address + remote path).
 *
 * `record` is only used to pre-fill the wizard when editing an existing library.
 */
const CloudPathFields = ({ record, canEditPath = true, helperText }) => {
  const translate = useTranslate()
  const notify = useNotify()
  const [testing, setTesting] = useState(false)
  const [result, setResult] = useState(null)

  const defaults = useMemo(
    () =>
      isCloudPath(record?.path)
        ? parseCloudPath(record.path)
        : { address: '', remotePath: '' },
    [record?.path],
  )

  const testConnection = useCallback(
    async (address, remotePath) => {
      setTesting(true)
      setResult(null)
      try {
        const res = await fetch('/api/cloudsource/test', {
          method: 'POST',
          headers: authHeaders(),
          body: JSON.stringify({ url: address, path: remotePath }),
        })
        if (res.status === 401 || res.status === 403) {
          setResult({
            ok: false,
            message: translate('resources.library.cloud.forbidden'),
          })
          return
        }
        const body = await res.json()
        setResult(body)
      } catch (error) {
        setResult({
          ok: false,
          message: error.message || String(error),
        })
      } finally {
        setTesting(false)
      }
    },
    [translate],
  )

  return (
    <FormDataConsumer>
      {({ formData }) => {
        const sourceType =
          formData.sourceType ||
          (isCloudPath(formData.path) ? SOURCE_TYPES.cloud : SOURCE_TYPES.local)
        const isCloud = sourceType === SOURCE_TYPES.cloud
        const address = formData.cloudAddress || defaults.address
        const remotePath = formData.cloudRemotePath || defaults.remotePath

        return (
          <>
            <RadioButtonGroupInput
              source="sourceType"
              label={translate('resources.library.fields.sourceType')}
              defaultValue={sourceType}
              variant="outlined"
              choices={[
                {
                  id: SOURCE_TYPES.local,
                  name: translate('resources.library.fields.sourceLocal'),
                },
                {
                  id: SOURCE_TYPES.cloud,
                  name: translate('resources.library.fields.sourceCloud'),
                },
              ]}
            />

            {!isCloud && (
              <TextInput
                source="path"
                label={translate('resources.library.fields.path')}
                validate={[required()]}
                fullWidth
                variant="outlined"
                InputProps={{ readOnly: !canEditPath }}
                helperText={canEditPath ? helperText : ''}
              />
            )}

            {isCloud && (
              <>
                <TextInput
                  source="cloudAddress"
                  label={translate('resources.library.fields.openlistAddress')}
                  validate={[required()]}
                  fullWidth
                  variant="outlined"
                  spellCheck={false}
                  InputProps={{ readOnly: !canEditPath }}
                  defaultValue={defaults.address}
                  placeholder={translate(
                    'resources.library.cloud.placeholderAddress',
                  )}
                  helperText={translate('resources.library.cloud.addressHelper')}
                />
                <TextInput
                  source="cloudRemotePath"
                  label={translate('resources.library.fields.remotePath')}
                  validate={[required()]}
                  fullWidth
                  variant="outlined"
                  spellCheck={false}
                  InputProps={{ readOnly: !canEditPath }}
                  defaultValue={defaults.remotePath}
                  placeholder={translate(
                    'resources.library.cloud.placeholderPath',
                  )}
                  helperText={translate('resources.library.cloud.pathHelper')}
                />

                <Box mt={1} mb={1}>
                  <Button
                    variant="outlined"
                    color="primary"
                    disabled={testing || !address || !canEditPath}
                    onClick={() => testConnection(address, remotePath)}
                  >
                    {testing
                      ? translate('resources.library.cloud.testing')
                      : translate('resources.library.cloud.test')}
                  </Button>
                </Box>

                {result && (
                  <Box mb={1}>
                    <Typography
                      variant="body2"
                      style={{
                        color: result.ok ? '#2e7d32' : '#c62828',
                        whiteSpace: 'pre-wrap',
                      }}
                    >
                      {result.ok
                        ? translate('resources.library.cloud.ok', {
                            count: (result.entries || []).length,
                            entries: (result.entries || [])
                              .map((e) => e.name)
                              .join('、'),
                          })
                        : translate('resources.library.cloud.failed', {
                            message: result.message || '',
                          })}
                    </Typography>
                  </Box>
                )}
              </>
            )}
          </>
        )
      }}
    </FormDataConsumer>
  )
}

export default CloudPathFields
