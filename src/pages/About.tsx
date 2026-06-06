import { FC, useLayoutEffect, useState } from "react";
import { DialogBody, DialogControlsSection, DialogControlsSectionHeader, Field, Navigation } from "@decky/ui";
import { FiGithub } from "react-icons/fi";
import { FaTelegram, FaHeart } from "react-icons/fa";
import { t } from 'i18next';
import { L } from "../i18n";
import { backend, ResourceType } from "../backend";
import { BRAND } from "../branding";
import { DescriptionField } from "../components";

export const About: FC = () => {
  const [version, setVersion] = useState<string>();
  const [coreVersion, setCoreVersion] = useState<string>();

  useLayoutEffect(() => {
    backend.getVersion(ResourceType.PLUGIN).then((x) => {
      setVersion(x);
    });
    backend.getVersion(ResourceType.CORE).then((x) => {
      setCoreVersion(x);
    });
  }, []);
  return (
    // The outermost div is to push the content down into the visible area
    <DialogBody>
      <DialogControlsSection>
        <DescriptionField label={BRAND.name}>
          Light-weight Clash/Mihomo proxy client for Steam OS.
        </DescriptionField>
        <Field
          label={t(L.INSTALLED_VERSION)} focusable >
          {version}
        </Field>
        <Field
          icon={<FiGithub style={{ display: "block" }} />}
          label="GitHub"
          onClick={() => {
            Navigation.NavigateToExternalWeb(BRAND.links.github);
          }}
        >
          GitHub Repo
        </Field>
      </DialogControlsSection>
      <DialogControlsSection>
        <DialogControlsSectionHeader>
          {t(L.COMMUNITY)}
        </DialogControlsSectionHeader>
        {BRAND.links.boosty && (
          <Field
            icon={<FaHeart style={{ display: "block" }} />}
            label={t(L.BOOSTY)}
            onClick={() => Navigation.NavigateToExternalWeb(BRAND.links.boosty)}
          >
            Boosty
          </Field>
        )}
        {BRAND.links.tgGames && (
          <Field
            icon={<FaTelegram style={{ display: "block" }} />}
            label={t(L.TG_GAMES)}
            onClick={() => Navigation.NavigateToExternalWeb(BRAND.links.tgGames)}
          >
            Telegram
          </Field>
        )}
        {BRAND.links.tgNews && (
          <Field
            icon={<FaTelegram style={{ display: "block" }} />}
            label={t(L.TG_NEWS)}
            onClick={() => Navigation.NavigateToExternalWeb(BRAND.links.tgNews)}
          >
            Telegram
          </Field>
        )}
        {BRAND.links.tgChat && (
          <Field
            icon={<FaTelegram style={{ display: "block" }} />}
            label={t(L.TG_CHAT)}
            onClick={() => Navigation.NavigateToExternalWeb(BRAND.links.tgChat)}
          >
            Telegram
          </Field>
        )}
      </DialogControlsSection>
      <DialogControlsSection>
        <DialogControlsSectionHeader>
          {t(L.DEPENDENCY)}
        </DialogControlsSectionHeader>
        <DescriptionField label="Mihomo">
          Another Mihomo Kernel.
          <br />
          <i>{BRAND.name} is powered by Mihomo.</i>
        </DescriptionField>
        <Field
          label={t(L.INSTALLED_VERSION)}
          focusable={true}
        >
          {coreVersion}
        </Field>
        <Field
          icon={<FiGithub style={{ display: "block" }} />}
          label="MetaCubeX/mihomo"
          onClick={() => {
            Navigation.NavigateToExternalWeb(
              "https://github.com/MetaCubeX/mihomo/tree/Meta"
            );
          }}
        >
          GitHub Repo
        </Field>
      </DialogControlsSection>
    </DialogBody>
  );
};
