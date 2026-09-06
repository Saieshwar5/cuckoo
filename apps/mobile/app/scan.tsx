import { CameraView, useCameraPermissions } from 'expo-camera';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Platform, StyleSheet, Text, View } from 'react-native';

import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { radius, spacing, type, useStyles, type Theme } from '@/theme';
import { parsePairCode } from '@/util/pairing';

// Scan: the camera, and a field to paste a link into for anyone without
// one, which is also how the browser gets by.
export default function ScanScreen() {
  const router = useRouter();
  const styles = useStyles(makeStyles);
  const [permission, requestPermission] = useCameraPermissions();
  const [pasted, setPasted] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [handled, setHandled] = useState(false);

  const open = (raw: string) => {
    const code = parsePairCode(raw);
    if (!code) {
      setError(t('scan.invalid'));
      return;
    }
    setHandled(true);
    router.replace({ pathname: '/p/[code]', params: { code } });
  };

  const cameraUsable = Platform.OS !== 'web' && permission?.granted;
  return (
    <Screen padded={false}>
      <TopBar title={t('scan.title')} onBack={() => router.back()} />
      <View style={styles.body}>
        {cameraUsable ? (
          <View style={styles.viewfinder}>
            <CameraView
              style={StyleSheet.absoluteFill}
              facing="back"
              barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
              onBarcodeScanned={handled ? undefined : ({ data }) => open(data)}
            />
            <View style={styles.frame} pointerEvents="none" />
          </View>
        ) : (
          <View style={styles.noCamera}>
            <Text style={styles.noCameraText}>
              {Platform.OS === 'web' ? t('scan.nocamera') : t('scan.permission')}
            </Text>
            {Platform.OS !== 'web' && permission && !permission.granted ? (
              <Button
                title={t('scan.allow')}
                onPress={() => void requestPermission()}
                variant="outline"
                compact
              />
            ) : null}
          </View>
        )}
        <Text style={styles.hint}>{cameraUsable ? t('scan.hint') : ''}</Text>
        <TextField
          label={t('scan.paste.label')}
          placeholder={t('scan.paste.placeholder')}
          value={pasted}
          onChangeText={(v) => {
            setPasted(v);
            setError(null);
          }}
          autoCapitalize="none"
          autoCorrect={false}
          icon="link-outline"
          error={error}
          returnKeyType="go"
          onSubmitEditing={() => open(pasted)}
          testID="paste-link"
        />
        <Button
          title={t('scan.open')}
          onPress={() => open(pasted)}
          disabled={!pasted.trim()}
          testID="open-link"
        />
      </View>
    </Screen>
  );
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { flex: 1, padding: spacing.lg, gap: spacing.md },
    viewfinder: { height: 300, borderRadius: radius.lg, overflow: 'hidden', backgroundColor: '#000' },
    frame: {
      position: 'absolute',
      left: '15%',
      right: '15%',
      top: '15%',
      bottom: '15%',
      borderWidth: 2,
      borderColor: '#FFFFFF',
      borderRadius: radius.md,
    },
    noCamera: {
      height: 200,
      borderRadius: radius.lg,
      backgroundColor: colors.surface,
      alignItems: 'center',
      justifyContent: 'center',
      gap: spacing.md,
      padding: spacing.lg,
    },
    noCameraText: { ...type.body, color: colors.textSecondary, textAlign: 'center' },
    hint: { ...type.caption, color: colors.textSecondary, textAlign: 'center' },
  });
