import { useId } from "react";
import { Flex, Text, TextField } from "@radix-ui/themes";
import type { useInlineReauthentication } from "@/hooks/useInlineReauthentication";

export function InlineReauthentication({ auth, disabled = false }: {
  auth: ReturnType<typeof useInlineReauthentication>;
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <Flex direction="column" gap="2" my="3">
      <Text as="label" htmlFor={id} size="2" weight="medium">
        {auth.twoFactor ? "身份验证 · 两步验证码" : "身份验证 · 当前密码"}
      </Text>
      <TextField.Root
        id={id}
        type="password"
        autoComplete={auth.twoFactor ? "one-time-code" : "current-password"}
        inputMode={auth.twoFactor ? "numeric" : undefined}
        placeholder={auth.twoFactor ? "输入验证器中的验证码" : "输入当前账户密码"}
        value={auth.value}
        onChange={(event) => auth.setValue(event.target.value)}
        disabled={disabled || !auth.available}
        aria-invalid={Boolean(auth.error)}
        aria-describedby={auth.error ? `${id}-error` : undefined}
      />
      {!auth.available && <Text size="2" color="gray">账户信息尚未就绪，请稍后重试或刷新页面。</Text>}
      {auth.error && <Text id={`${id}-error`} size="2" color="red" role="alert">{auth.error}</Text>}
    </Flex>
  );
}
