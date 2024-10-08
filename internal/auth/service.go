package nynAuth

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"golang.org/x/text/encoding/simplifiedchinese"
	"log"
	"net"
	"nyn/internal/crypto"
)

// DeviceInterface defines methods for sending and receiving packets.
// It is implemented by Package B (Device).
type DeviceInterface interface {
	Send(l ...gopacket.SerializableLayer) ([]byte, error)
	SetBPFFilter(f string, a ...any) (string, error)
	GetLocalMAC() net.HardwareAddr
	GetTargetMAC() net.HardwareAddr
	SetTargetMAC(mac net.HardwareAddr)
}

type AuthService struct {
	device    DeviceInterface
	h3cInfo   nynCrypto.H3CInfo
	h3cBuffer []byte
	username  string
	password  string
}

// NewPacketHandler creates a new PacketHandler that depends on a device
func New(device DeviceInterface, h3cInfo nynCrypto.H3CInfo, username string, password string) *AuthService {
	return &AuthService{
		device:   device,
		h3cInfo:  h3cInfo,
		username: username,
		password: password,
	}
}

func (as *AuthService) HandlePacket(packet gopacket.Packet) error {
	//log.Println("nyn - received - ", packet.Data())
	ethLayer := packet.Layer(layers.LayerTypeEthernet)
	ethPacket, _ := ethLayer.(*layers.Ethernet)

	if eapLayer := packet.Layer(layers.LayerTypeEAP); eapLayer != nil {
		eapPacket, _ := eapLayer.(*layers.EAP)
		log.Printf("h3c - server - [%d](%d)<%d>\n", eapPacket.Id, eapPacket.Type, eapPacket.Code)

		if as.device.GetTargetMAC() == nil {
			log.Println("h3c - server - asked first identity")
			as.device.SetTargetMAC(ethPacket.SrcMAC)
			as.device.SetBPFFilter("ether src %s and ether proto 0x888E", ethPacket.SrcMAC)
			as.SendFirstIdentity(eapPacket.Id)
			log.Println("nyn - client - answered first identity")
			return nil // return func to avoid proceed to following logic
		}

		switch eapPacket.Code {
		case layers.EAPCodeSuccess:
			log.Println("nyn - client - suc (^_^)")
		case layers.EAPCodeFailure:
			if eapPacket.Type == EAPTypeMD5Failed {
				failMsgSize := eapPacket.TypeData[0]
				failMsg, _ := simplifiedchinese.GBK.NewDecoder().Bytes(eapPacket.TypeData[1 : failMsgSize-1])
				log.Printf("nyn - server - %s\n", failMsg)
				log.Fatal("nyn - client = fal (o.0)")
			} else {
				log.Fatal("nyn - client = maybe we should re-auth?")
			}
		case layers.EAPCodeRequest:
			log.Println("h3c - server - asking...")
		case EAPCodeH3CData:
			if eapPacket.TypeData[H3CIntegrityChanllengeHeader-1] == 0x35 {
				log.Println("h3c - server - integrity challange")
				var err error
				as.h3cBuffer, err = as.h3cInfo.ChallangeResponse(
					eapPacket.TypeData[H3CIntegrityChanllengeHeader:][:H3CIntegrityChanllengeLength])
				if err != nil {
					log.Fatal("nyn - client - ", err)
				}
				log.Println("nyn - client - integrity set")
			}
		default:
			log.Println("nyn - client - unknow eap code ^ ")
		}

		switch eapPacket.Type {
		case layers.EAPTypeNone:
			log.Println("h3c - server - suc or fal")
		case layers.EAPTypeOTP:
			log.Println("h3c - server - asked md5otp")
			as.SendResponseMD5(eapPacket.Id, eapPacket.Contents)
			log.Println("nyn - client - answered md5otp")
		case layers.EAPTypeIdentity:
			log.Println("h3c - server - asked identity")
		default:
			log.Println("nyn - client - unknow eap type ^ ")
		}
	}

	return nil
}
