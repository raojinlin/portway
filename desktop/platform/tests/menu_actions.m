#include <assert.h>
#include "../menu_darwin.m"

static NSString *lastName;
static BOOL lastEnabled;
void stmMenuAction(int action) {}
void stmTunnelAction(const char *name, int enabled) {
    lastName = [NSString stringWithUTF8String:name];
    lastEnabled = enabled != 0;
}

int main(void) {
    @autoreleasepool {
        menuTarget = [STMMenuTarget new];
        NSMenu *menu = [NSMenu new];
        menu.autoenablesItems = NO;
        [menu addItem:[[NSMenuItem alloc] initWithTitle:@"Details" action:nil keyEquivalent:@""]];
        NSDictionary *labels = @{@"startTunnel": @"Start Tunnel", @"stopTunnel": @"Stop Tunnel", @"working": @"Working...",
            @"copyProxy": @"Copy Proxy URL", @"copyCommand": @"Copy Proxy Command"};
        NSMutableDictionary *line = [@{@"name": @"line / test", @"enabled": @NO, @"busy": @NO} mutableCopy];
        stmUpdateLineActions(menu, line, labels);
        assert(menu.numberOfItems == 3);
        NSMenuItem *toggle = [menu itemAtIndex:2];
        assert([toggle.title isEqualToString:@"Start Tunnel"] && toggle.enabled);
        [menuTarget setTunnel:toggle];
        assert([lastName isEqualToString:line[@"name"]] && lastEnabled && !toggle.enabled);

        line[@"enabled"] = @YES;
        line[@"proxyURL"] = @"socks5h://127.0.0.1:1080";
        line[@"proxyCommand"] = @"export ALL_PROXY='socks5h://127.0.0.1:1080'";
        stmUpdateLineActions(menu, line, labels);
        assert(menu.numberOfItems == 5);
        assert([toggle.title isEqualToString:@"Stop Tunnel"] && toggle.enabled);
        assert([[[menu itemAtIndex:3] representedObject] isEqual:line[@"proxyURL"]]);
        [menuTarget setTunnel:toggle];
        assert(!lastEnabled);

        line[@"busy"] = @YES;
        stmUpdateLineActions(menu, line, labels);
        assert([toggle.title isEqualToString:@"Working..."] && !toggle.enabled);
        line[@"busy"] = @NO;
        line[@"enabled"] = @NO;
        [line removeObjectForKey:@"proxyURL"];
        [line removeObjectForKey:@"proxyCommand"];
        stmUpdateLineActions(menu, line, labels);
        assert(menu.numberOfItems == 3 && [toggle.title isEqualToString:@"Start Tunnel"]);
        puts("Menu actions: start, stop, busy and proxy visibility OK.");
    }
    return 0;
}
