package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/daos"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/errs"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/helpers"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/models"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/serializers"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/delegate_cash"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/hiro"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/lighthouse"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/magiceden"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jinzhu/gorm"
)

func (s *Service) GetModelDefaultByChainID(chainID uint64) string {
	var mapModelNameToModelID = models.MAP_CHAIN_ID_TO_LLM_MODEL[chainID]
	for modelName, _ := range mapModelNameToModelID {
		return modelName
	}
	return "NousResearch/Hermes-3-Llama-3.1-70B-FP8"
}

func (s *Service) AgentCreateAgentAssistant(ctx context.Context, address string, req *serializers.AssistantsReq) (*models.AgentInfo, error) {
	//if req.SystemContent == "" {
	//	return nil, errs.NewError(errs.ErrBadRequest)
	//}
	agent := &models.AgentInfo{
		Version:          "2",
		AgentType:        models.AgentInfoAgentTypeReasoning,
		AgentID:          helpers.RandomBigInt(12).Text(16),
		Status:           models.AssistantStatusPending,
		NetworkID:        req.ChainID,
		NetworkName:      models.GetChainName(req.ChainID),
		AgentName:        req.AgentName,
		Creator:          strings.ToLower(address),
		MetaData:         req.SystemContent,
		SystemPrompt:     req.SystemContent,
		AgentBaseModel:   req.AgentBaseModel,
		Bio:              req.GetAssistantCharacter(req.Bio),
		Lore:             req.GetAssistantCharacter(req.Lore),
		Knowledge:        req.GetAssistantCharacter(req.Knowledge),
		MessageExamples:  req.GetAssistantCharacter(req.MessageExamples),
		PostExamples:     req.GetAssistantCharacter(req.PostExamples),
		Topics:           req.GetAssistantCharacter(req.Topics),
		Style:            req.GetAssistantCharacter(req.Style),
		Adjectives:       req.GetAssistantCharacter(req.Adjectives),
		SocialInfo:       req.GetAssistantCharacter(req.SocialInfo),
		AgentFee:         models.GetAgentFee(req.ChainID),
		ScanEnabled:      false,
		VerifiedNftOwner: req.VerifiedNFTOwner,
		NftAddress:       req.NFTAddress,
		NftTokenID:       req.NFTTokenID,
		NftOwnerAddress:  req.NFTOwnerAddress,
		Thumbnail:        req.Thumbnail,
		NftTokenImage:    req.NFTTokenImage,
	}
	if req.TokenChainId != "" {
		tokenChainId, _ := strconv.ParseUint(req.TokenChainId, 0, 64)
		if !(tokenChainId == models.POLYGON_CHAIN_ID || tokenChainId == models.ZKSYNC_CHAIN_ID) {
			agent.TokenNetworkID = tokenChainId
			if req.CreateTokenMode != "" {
				agent.TokenMode = string(req.CreateTokenMode)
			}
		} else {
			agent.TokenNetworkID = models.GENERTAL_NETWORK_ID
			agent.TokenMode = string(models.TokenSetupEnumNoToken)
		}
	}

	if agent.AgentBaseModel == "" {
		agent.AgentBaseModel = s.GetModelDefaultByChainID(req.ChainID)
	}
	if req.VerifiedNFTOwner {
		if req.NFTAddress == "" || req.NFTTokenID == "" || req.NFTOwnerAddress == "" {
			req.VerifiedNFTOwner = false
		} else {
			checked := false
			if strings.Contains(req.NFTTokenID, "i0") {
				// inscription
				checked = s.CheckOwnerInscription(req.NFTDelegateAddress, req.NFTOwnerAddress, req.NFTPublicKey, req.NFTTokenID, req.NFTSignature, req.NFTSignMessage)
			} else {
				checked = s.CheckOwnerNFT721(req.NFTDelegateAddress, req.NFTOwnerAddress, req.NFTAddress, req.NFTTokenID, req.NFTSignature, req.NFTSignMessage)
			}
			agent.VerifiedNftOwner = checked
		}
	}

	if req.TwinTwitterUsernames != "" {
		agent.TwinTwitterUsernames = req.TwinTwitterUsernames
		agent.TwinStatus = models.TwinStatusPending
		listPendingAgents, err := s.dao.FindAgentInfo(
			daos.GetDBMainCtx(ctx),
			map[string][]interface{}{
				`twin_twitter_usernames != '' and twin_status = ?`: {models.TwinStatusPending},
				"scan_enabled = ?": {true},
			},
			map[string][]interface{}{},
			[]string{
				"id asc",
			},
			0,
			999999,
		)
		if err != nil {
			return nil, err
		}
		estimateDoneTime := time.Now().Add(time.Duration(len(listPendingAgents)) * 30 * time.Minute)
		agent.EstimateTwinDoneTimestamp = &estimateDoneTime
	}

	//generate address
	{
		ethAddress, err := s.CreateETHAddress(ctx)
		if err != nil {
			return nil, errs.NewError(err)
		}
		agent.ETHAddress = strings.ToLower(ethAddress)

		solAddress, err := s.CreateSOLAddress(ctx)
		if err != nil {
			return nil, errs.NewError(err)
		}
		agent.SOLAddress = solAddress

		addressBtc, err := s.CreateBTCAddress(ctx)
		if err != nil {
			return nil, errs.NewError(err)
		}
		agent.TipBtcAddress = addressBtc

		addressEth, err := s.CreateETHAddress(ctx)
		if err != nil {
			return nil, errs.NewError(err)
		}
		agent.TipEthAddress = addressEth

		addressSol, err := s.CreateSOLAddress(ctx)
		if err != nil {
			return nil, errs.NewError(err)
		}
		agent.TipSolAddress = addressSol
	}

	err := s.dao.Create(daos.GetDBMainCtx(ctx), agent)
	if err != nil {
		return nil, errs.NewError(err)
	}

	agentTokenInfo := &models.AgentTokenInfo{}
	agentTokenInfo.AgentInfoID = agent.ID
	agentTokenInfo.NetworkID = agent.TokenNetworkID
	agentTokenInfo.NetworkName = models.GetChainName(agent.TokenNetworkID)
	err = s.dao.Create(daos.GetDBMainCtx(ctx), agentTokenInfo)
	if err != nil {
		return nil, errs.NewError(err)
	}

	agent.TokenInfoID = agentTokenInfo.ID
	err = s.dao.Save(daos.GetDBMainCtx(ctx), agent)
	if err != nil {
		return nil, errs.NewError(err)
	}

	if agent.ID > 0 {
		go s.AgentCreateMissionDefault(context.Background(), agent.ID)
	}

	return agent, nil
}

func (s *Service) CheckOwnerInscription(delegate, vault, publicKey, tokenId, signature, signMessage string) bool {
	if !strings.Contains(tokenId, "i0") {
		return false
	}
	if signature != "" {
		temp, err := s.verifyInscription(signature, delegate, publicKey, signMessage)
		if err != nil || !temp {
			return false
		}
	}
	owner := ""
	magicEdenService := magiceden.NewMagicedenService()
	item, err := magicEdenService.GetInscriptionInfo(tokenId)
	if err == nil {
		owner = item.Owner
	} else {
		service := hiro.NewHiroService(s.conf.HiroUrl)
		info, err := service.GetInscriptionInfo(tokenId)
		if err == nil {
			owner = info.Address
		}
	}
	if owner == "" {
		return false
	}
	if strings.ToLower(delegate) == strings.ToLower(vault) && strings.ToLower(owner) == strings.ToLower(delegate) {
		return true
	}
	return false
}

func (s *Service) CheckOwnerNFT721(delegate, vault, contract, tokenId, signature, signMessage string) bool {
	if strings.Contains(tokenId, "i0") {
		return false
	}
	if signature != "" {
		temp, _ := s.verify(signature, delegate, signMessage)
		if !temp {
			return false
		}
	}

	if strings.ToLower(delegate) == strings.ToLower(vault) {
		//return true
	}

	chainID := 1 //default ETH
	hardcodeCollection := s.openseaService.FindHardCodeCollectionByAddress(contract)
	if hardcodeCollection != nil {
		chainID = hardcodeCollection.ChainID
	}

	delegateCash := delegate_cash.NewDelegateCashAPIService(s.conf.DelegateCash.Url, s.conf.DelegateCash.ApiKey)
	checked, err := delegateCash.CheckDelegateForTokenERC721V1(delegate, vault, contract, tokenId, chainID)
	if err != nil {
		fmt.Println(err)
	}
	if !checked {
		checked, err = delegateCash.CheckDelegateForTokenERC721V2(delegate, vault, contract, tokenId, chainID)
		if err != nil {
			fmt.Println(err)
		}
	}
	return checked
}

func (s *Service) verify(signatureHex string, signer string, msgStr string) (bool, error) {
	_, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, err
	}
	sig := hexutil.MustDecode(signatureHex)

	msgBytes := []byte(msgStr)
	msgHash := accounts.TextHash(msgBytes)

	if sig[crypto.RecoveryIDOffset] == 27 || sig[crypto.RecoveryIDOffset] == 28 {
		sig[crypto.RecoveryIDOffset] -= 27 // Transform yellow paper V from 27/28 to 0/1
	}

	recovered, err := crypto.SigToPub(msgHash, sig)
	if err != nil {
		return false, err
	}

	recoveredAddr := crypto.PubkeyToAddress(*recovered)
	signerHex := recoveredAddr.Hex()
	isVerified := strings.ToLower(signer) == strings.ToLower(signerHex)

	return isVerified, nil
}

func (s *Service) verifyInscription(signatureHex string, signer, publicKey string, msgStr string) (bool, error) {
	fullUrl := "https://api-verify-sig.eternalai.org/api/unisat/verify-sig"
	req := map[string]string{
		"address":   signer,
		"pubKey":    publicKey,
		"message":   msgStr,
		"signature": signatureHex,
	}
	resp, _, i, err := helpers.HttpRequest(fullUrl, "POST",
		map[string]string{
			"Content-Type": "application/json",
		},
		req)
	if err != nil {
		return false, err
	}
	if i != http.StatusOK && i != http.StatusCreated {
		return false, nil
	}
	return string(resp) == "true", nil
}

func (s *Service) AgentUpdateAgentAssistant(ctx context.Context, address string, req *serializers.AssistantsReq) (*models.AgentInfo, error) {
	var agent *models.AgentInfo
	err := daos.WithTransaction(
		daos.GetDBMainCtx(ctx),
		func(tx *gorm.DB) error {
			var err error
			if req.AgentID != "" {
				agent, err = s.dao.FirstAgentInfo(tx,
					map[string][]interface{}{
						"agent_id = ?": {req.AgentID},
					},
					map[string][]interface{}{}, []string{})
				if err != nil {
					return errs.NewError(err)
				}

				if agent != nil {
					agent, err = s.dao.FirstAgentInfoByID(tx, agent.ID, map[string][]interface{}{}, true)
					if err != nil {
						return errs.NewError(err)
					}
				}
			} else {
				agent, err = s.dao.FirstAgentInfoByID(tx, req.ID, map[string][]interface{}{}, true)
				if err != nil {
					return errs.NewError(err)
				}
			}

			if agent != nil {
				if !strings.EqualFold(agent.Creator, address) {
					return errs.NewError(errs.ErrInvalidOwner)
				}

				if !(agent.AgentContractID != "" || agent.AgentNftMinted == true) {
					agent.NetworkID = req.ChainID
					agent.NetworkName = models.GetChainName(req.ChainID)
				}

				if req.AgentName != "" {
					agent.AgentName = req.AgentName
				}

				if req.SystemContent != "" {
					agent.MetaData = req.SystemContent
					agent.SystemPrompt = req.SystemContent
				}

				agent.Bio = req.GetAssistantCharacter(req.Bio)
				agent.Lore = req.GetAssistantCharacter(req.Lore)
				agent.Knowledge = req.GetAssistantCharacter(req.Knowledge)
				agent.MessageExamples = req.GetAssistantCharacter(req.MessageExamples)
				agent.PostExamples = req.GetAssistantCharacter(req.PostExamples)
				agent.Topics = req.GetAssistantCharacter(req.Topics)
				agent.Style = req.GetAssistantCharacter(req.Style)
				agent.Adjectives = req.GetAssistantCharacter(req.Adjectives)
				agent.SocialInfo = req.GetAssistantCharacter(req.SocialInfo)

				if agent.TokenStatus == "" && agent.TokenAddress == "" {
					if req.TokenChainId != "" {
						tokenChainId, _ := strconv.ParseUint(req.TokenChainId, 0, 64)
						if !(tokenChainId == models.POLYGON_CHAIN_ID || tokenChainId == models.ZKSYNC_CHAIN_ID) {
							agent.TokenNetworkID = tokenChainId
							agent.TokenSymbol = req.Ticker
							if req.CreateTokenMode != "" {
								agent.TokenMode = string(req.CreateTokenMode)
							}

							if agent.TokenMode == string(models.TokenSetupEnumAutoCreate) && agent.AgentNftMinted {
								agent.TokenStatus = "pending"
							}
						} else {
							agent.TokenNetworkID = models.GENERTAL_NETWORK_ID
							agent.TokenMode = string(models.TokenSetupEnumNoToken)
						}
					}
				}

				err := s.dao.Save(tx, agent)
				if err != nil {
					return errs.NewError(err)
				}

				go s.AgentCreateMissionDefault(context.Background(), agent.ID)
			}
			return nil
		},
	)

	if err != nil {
		return nil, errs.NewError(err)
	}

	return agent, nil
}

func (s *Service) UpdateAgentInfoInContract(ctx context.Context, address string, req *serializers.UpdateAgentAssistantInContractRequest) (*models.AgentInfo, error) {
	var agent *models.AgentInfo
	err := daos.WithTransaction(
		daos.GetDBMainCtx(ctx),
		func(tx *gorm.DB) error {
			var err error
			if req.AgentID != "" {
				agent, err = s.dao.FirstAgentInfo(tx,
					map[string][]interface{}{
						"agent_id = ?": {req.AgentID},
					},
					map[string][]interface{}{}, []string{})
				if err != nil {
					return errs.NewError(err)
				}

				if agent != nil {
					agent, err = s.dao.FirstAgentInfoByID(tx, agent.ID, map[string][]interface{}{}, true)
					if err != nil {
						return errs.NewError(err)
					}
				}
			} else {
				agent, err = s.dao.FirstAgentInfoByID(tx, req.ID, map[string][]interface{}{}, true)
				if err != nil {
					return errs.NewError(err)
				}
			}

			if agent != nil {
				if !strings.EqualFold(agent.Creator, address) {
					return errs.NewError(errs.ErrInvalidOwner)
				}

				systemPromptBytes, _, err := lighthouse.DownloadDataSimple(req.HashSystemPrompt)
				if err != nil {
					return errs.NewError(err)
				}
				dataBytes, _, err := lighthouse.DownloadDataSimple(req.HashName)
				if err != nil {
					return errs.NewError(err)
				}
				agentUriData := models.AgentUriData{}
				err = json.Unmarshal(dataBytes, &agentUriData)
				if err != nil {
					return errs.NewError(err)
				}

				agent.SystemPrompt = string(systemPromptBytes)
				agent.AgentName = agentUriData.Name
				agent.Uri = req.HashName
				err = s.dao.Save(tx, agent)
				if err != nil {
					return errs.NewError(err)
				}

				_, err = s.ExecuteUpdateAgentInfoInContract(ctx, agent, req)
				if err != nil {
					return errs.NewError(err)
				}
			}
			return nil
		},
	)

	if err != nil {
		return nil, errs.NewError(err)
	}

	return agent, nil
}

func (s *Service) UploadDataToLightHouse(ctx context.Context, address string, req *serializers.DataUploadToLightHouse) (string, error) {
	hash, err := lighthouse.UploadData(s.conf.Lighthouse.Apikey, address, []byte(req.Content))
	if err != nil {
		return "", errs.NewError(err)
	}
	return fmt.Sprintf("ipfs://%v", hash), nil
}
