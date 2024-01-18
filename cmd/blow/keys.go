package main

/*func WaitForKeysSync(ctx context.Context, config *NodeConfig) (*admin.Administration, crypto.PrivateKey) {
nodeToken := crypto.TokenFromString(config.Token)
token, pk := crypto.RandomAsymetricKey()
fmt.Printf("waiting for secrete key for token\n\n        %v\n\non admin port %v with provisory token\n\n        %v\n\n", nodeToken, config.AdminPort, token)
administration := &admin.Administration{
	Firewall:       socket.ValidateSingleConnection(nodeToken),
	Secret:         pk,
	Port:           config.AdminPort,
	Status:         make(chan chan string),
	Activation:     make(chan admin.Activation),
	FirewallAction: make(chan admin.FirewallAction),
}*/
/*go func() {
	time.Sleep(1 * time.Second)
	bytes, _ := hex.DecodeString("157acd1830b2b35ea02d8f5e0745730c5dfedfc8b0d734858fcf49ca684393ecdbc43a4695df777ea27f9699fbf346b2ff259f9c90815d864a9c98b4c787cf17")
	tokenAdrr := socket.TokenAddr{
		Token: token,
		Addr:  ":5403",
	}
	var pk crypto.PrivateKey
	copy(pk[:], bytes)
	admin, err := admin.DialAdmin("", tokenAdrr, pk)
	if err != nil {
		fmt.Println(err)
		return
	}
	admin.SendSecret(pk)
}()
*/
/*	nodeSecret, err := administration.WaitForKeys(ctx, nodeToken)
	if err != nil {
		slog.Info("could not get secret key for node: %v", err)
		return nil, crypto.ZeroPrivateKey
	}

	return administration, nodeSecret
}
*/

/*
func WaitForKeys(ctx context.Context, dh chan crypto.PrivateKey, status chan chan string, tokens ...crypto.Token) chan []crypto.PrivateKey {
	completed := make(chan []crypto.PrivateKey)
	secrets := make([]crypto.PrivateKey, 0)
	go func() {
		for {
			select {
			case <-ctx.Done():
				completed <- nil
				return
			case pk := <-dh:
				tk := pk.PublicKey()
				fmt.Println("new key for", tk)
				for i, token := range tokens {
					if token == tk {
						secrets = append(secrets, pk)
						tokens = append(tokens[:i], tokens[i+1:]...)
						if len(tokens) == 0 {
							completed <- secrets
							return
						}
					}
				}
			case req := <-status:
				req <- fmt.Sprintf("waiting for %v more keys", len(tokens))
			}

		}
	}()
	return completed
}

func main2() {
	socket.TCPNetworkTest.AddNode("server", 1.0, 10*time.Millisecond, 1e9)
	socket.TCPNetworkTest.AddNode("client", 1.0, 10*time.Millisecond, 1e9)

	tk, pk := crypto.RandomAsymetricKey()
	tk1, pk1 := crypto.RandomAsymetricKey()
	tk2, pk2 := crypto.RandomAsymetricKey()
	_, cpk := crypto.RandomAsymetricKey()
	ctx := context.Background()
	dh := make(chan crypto.PrivateKey)
	status := make(chan chan string)
	server := admin.AdminServer{
		Hostname:      "server",
		Firewall:      socket.AcceptAllConnections,
		Secret:        pk,
		Port:          6000,
		Status:        status,
		DiffieHellman: dh,
	}
	server.Start(ctx)

	go func() {
		time.Sleep(1000 * time.Millisecond)
		client, err := admin.DialAdmin("client", socket.TokenAddr{Addr: "server:6000", Token: tk}, cpk)
		if err != nil {
			panic(err)
		}
		status, err := client.Status()
		if err != nil {
			panic(err)
		}
		fmt.Println(status)
		time.Sleep(200 * time.Millisecond)
		err = client.SendSecret(pk1)
		if err != nil {
			panic(err)
		}
		time.Sleep(200 * time.Millisecond)
		err = client.SendSecret(pk2)
		if err != nil {
			panic(err)
		}
	}()
	pks := <-WaitForKeys(ctx, dh, status, tk1, tk2)

	fmt.Println(pk1)
	fmt.Println(pks[0])

	fmt.Println(pk2)
	fmt.Println(pks[1])

}
*/
